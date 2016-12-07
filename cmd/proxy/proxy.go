// Copyright 2015 Sorint.lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/flagutil"
	"github.com/sorintlab/stolon/pkg/store"

	"github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/pollon"
	"github.com/spf13/cobra"
	"github.com/uber-go/zap"
	"github.com/uber-go/zap/zwrap"
)

var log = zap.New(zap.NewTextEncoder(), zap.AddCaller())

var cmdProxy = &cobra.Command{
	Use: "stolon-proxy",
	Run: proxy,
}

type config struct {
	storeBackend       string
	storeEndpoints     string
	storeCertFile      string
	storeKeyFile       string
	storeCAFile        string
	storeSkipTlsVerify bool
	clusterName        string
	listenAddress      string
	port               string
	stopListening      bool
	debug              bool
}

var cfg config

func init() {
	cmdProxy.PersistentFlags().StringVar(&cfg.storeBackend, "store-backend", "", "store backend type (etcd or consul)")
	cmdProxy.PersistentFlags().StringVar(&cfg.storeEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (use https scheme for tls communication) (defaults: http://127.0.0.1:2379 for etcd, http://127.0.0.1:8500 for consul)")
	cmdProxy.PersistentFlags().StringVar(&cfg.storeCertFile, "store-cert-file", "", "certificate file for client identification to the store")
	cmdProxy.PersistentFlags().StringVar(&cfg.storeKeyFile, "store-key", "", "private key file for client identification to the store")
	cmdProxy.PersistentFlags().StringVar(&cfg.storeCAFile, "store-ca-file", "", "verify certificates of HTTPS-enabled store servers using this CA bundle")
	cmdProxy.PersistentFlags().BoolVar(&cfg.storeSkipTlsVerify, "store-skip-tls-verify", false, "skip store certificate verification (insecure!!!)")
	cmdProxy.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdProxy.PersistentFlags().StringVar(&cfg.listenAddress, "listen-address", "127.0.0.1", "proxy listening address")
	cmdProxy.PersistentFlags().StringVar(&cfg.port, "port", "5432", "proxy listening port")
	cmdProxy.PersistentFlags().BoolVar(&cfg.stopListening, "stop-listening", true, "stop listening on store error")
	cmdProxy.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
}

type ClusterChecker struct {
	id            string
	listenAddress string
	port          string

	stopListening bool

	listener         *net.TCPListener
	pp               *pollon.Proxy
	e                *store.StoreManager
	endPollonProxyCh chan error
}

func NewClusterChecker(id string, cfg config) (*ClusterChecker, error) {
	storePath := filepath.Join(common.StoreBasePath, cfg.clusterName)

	kvstore, err := store.NewStore(store.Config{
		Backend:       store.Backend(cfg.storeBackend),
		Endpoints:     cfg.storeEndpoints,
		CertFile:      cfg.storeCertFile,
		KeyFile:       cfg.storeKeyFile,
		CAFile:        cfg.storeCAFile,
		SkipTLSVerify: cfg.storeSkipTlsVerify,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}
	e := store.NewStoreManager(kvstore, storePath)

	return &ClusterChecker{
		id:               id,
		listenAddress:    cfg.listenAddress,
		port:             cfg.port,
		stopListening:    cfg.stopListening,
		e:                e,
		endPollonProxyCh: make(chan error),
	}, nil
}

func (c *ClusterChecker) startPollonProxy() error {
	if c.pp != nil {
		return nil
	}

	log.Info("Starting proxying")
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cfg.listenAddress, cfg.port))
	if err != nil {
		return fmt.Errorf("error resolving tcp addr %q: %v", addr.String(), err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("error listening on tcp addr %q: %v", addr.String(), err)
	}

	pp, err := pollon.NewProxy(listener)
	if err != nil {
		return fmt.Errorf("error creating pollon proxy: %v", err)
	}
	c.pp = pp
	c.listener = listener

	go func() {
		c.endPollonProxyCh <- c.pp.Start()
	}()

	return nil
}

func (c *ClusterChecker) stopPollonProxy() {
	if c.pp != nil {
		log.Info("Stopping listening")
		c.pp.Stop()
		c.pp = nil
		c.listener.Close()
		c.listener = nil
	}
}

func (c *ClusterChecker) sendPollonConfData(confData pollon.ConfData) {
	if c.pp != nil {
		c.pp.C <- confData
	}
}

func (c *ClusterChecker) SetProxyInfo(e *store.StoreManager, uid string, generation int64, ttl time.Duration) error {
	proxyInfo := &cluster.ProxyInfo{
		UID:             c.id,
		ProxyUID:        uid,
		ProxyGeneration: generation,
	}
	log.Debug("proxyInfo dump", zap.String("proxyInfo", spew.Sdump(proxyInfo)))

	if err := c.e.SetProxyInfo(proxyInfo, ttl); err != nil {
		return err
	}
	return nil
}

func (c *ClusterChecker) Check() error {
	cd, _, err := c.e.GetClusterData()
	if err != nil {
		log.Error("cannot get cluster data", zap.Error(err))
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		if c.stopListening {
			c.stopPollonProxy()
		}
		return nil
	}
	log.Debug("cd dump", zap.String("cd", spew.Sdump(cd)))
	if cd == nil {
		log.Info("no clusterdata available, closing connections to previous master")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return nil
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		log.Error("unsupported clusterdata format version", zap.Uint64("version", cd.FormatVersion))
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return nil
	}
	if err = cd.Cluster.Spec.Validate(); err != nil {
		log.Error("clusterdata validation failed", zap.Error(err))
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return nil
	}

	// Start pollon if not active
	if err = c.startPollonProxy(); err != nil {
		log.Error("failed to start proxy", zap.Error(err))
		return nil
	}

	proxy := cd.Proxy
	if proxy == nil {
		log.Info("no proxy object available, closing connections to previous master")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		if err = c.SetProxyInfo(c.e, proxy.UID, proxy.Generation, 2*cluster.DefaultProxyCheckInterval); err != nil {
			log.Error("failed to update proxyInfo", zap.Error(err))
		}
		return nil
	}

	db, ok := cd.DBs[proxy.Spec.MasterDBUID]
	if !ok {
		log.Info("no db object available, closing connections to previous master", zap.String("db", proxy.Spec.MasterDBUID))
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		if err = c.SetProxyInfo(c.e, proxy.UID, proxy.Generation, 2*cluster.DefaultProxyCheckInterval); err != nil {
			log.Error("failed to update proxyInfo", zap.Error(err))
		}
		return nil
	}

	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%s", db.Status.ListenAddress, db.Status.Port))
	if err != nil {
		log.Error("error", zap.Error(err))
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return nil
	}
	log.Info("master address", zap.Stringer("address", addr))
	if err = c.SetProxyInfo(c.e, proxy.UID, proxy.Generation, 2*cluster.DefaultProxyCheckInterval); err != nil {
		log.Error("failed to update proxyInfo", zap.Error(err))
	}

	c.sendPollonConfData(pollon.ConfData{DestAddr: addr})
	return nil
}

func (c *ClusterChecker) Start() error {
	endPollonProxyCh := make(chan error)
	checkCh := make(chan error)
	timerCh := time.NewTimer(0).C

	for true {
		select {
		case <-timerCh:
			go func() {
				checkCh <- c.Check()
			}()
		case err := <-checkCh:
			if err != nil {
				log.Debug("check reported error", zap.Error(err))
			}
			if err != nil {
				return fmt.Errorf("checker fatal error: %v", err)
			}
			timerCh = time.NewTimer(cluster.DefaultProxyCheckInterval).C
		case err := <-endPollonProxyCh:
			if err != nil {
				return fmt.Errorf("proxy error: %v", err)
			}
		}
	}
	return nil
}

func main() {
	flagutil.SetFlagsFromEnv(cmdProxy.PersistentFlags(), "STPROXY")

	cmdProxy.Execute()
}

func proxy(cmd *cobra.Command, args []string) {
	if cfg.debug {
		log.SetLevel(zap.DebugLevel)
	}
	stdlog, _ := zwrap.Standardize(log, zap.DebugLevel)
	pollon.SetLogger(stdlog)

	if cfg.clusterName == "" {
		fmt.Printf("cluster name required")
		os.Exit(1)
	}
	if cfg.storeBackend == "" {
		fmt.Printf("store backend type required")
		os.Exit(1)
	}

	id := common.UID()
	log.Info("proxy id", zap.String("id", id))

	clusterChecker, err := NewClusterChecker(id, cfg)
	if err != nil {
		fmt.Printf("cannot create cluster checker: %v", err)
		os.Exit(1)
	}
	clusterChecker.Start()
}
