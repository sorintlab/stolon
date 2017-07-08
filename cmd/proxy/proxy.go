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
	"strings"
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

func stderr(format string, a ...interface{}) {
	out := fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stderr, strings.TrimSuffix(out, "\n"))
}

func stdout(format string, a ...interface{}) {
	out := fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stdout, strings.TrimSuffix(out, "\n"))
}

func die(format string, a ...interface{}) {
	stderr(format, a...)
	os.Exit(1)
}

type ClusterChecker struct {
	uid           string
	listenAddress string
	port          string

	stopListening bool

	listener         *net.TCPListener
	pp               *pollon.Proxy
	e                *store.StoreManager
	endPollonProxyCh chan error
}

func NewClusterChecker(uid string, cfg config) (*ClusterChecker, error) {
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
		uid:              uid,
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
		UID:             c.uid,
		ProxyUID:        uid,
		ProxyGeneration: generation,
	}
	log.Debug("proxyInfo dump", zap.String("proxyInfo", spew.Sdump(proxyInfo)))

	if err := c.e.SetProxyInfo(proxyInfo, ttl); err != nil {
		return err
	}
	return nil
}

// Check reads the cluster data and apply the right pollon configuration.
func (c *ClusterChecker) Check() error {
	cd, _, err := c.e.GetClusterData()
	if err != nil {
		return fmt.Errorf("cannot get cluster data: %v", err)
	}

	// Start pollon if not active
	if err = c.startPollonProxy(); err != nil {
		return fmt.Errorf("failed to start proxy: %v", err)
	}

	log.Debug("cd dump", zap.String("cd", spew.Sdump(cd)))
	if cd == nil {
		log.Info("no clusterdata available, closing connections to master")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return nil
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return fmt.Errorf("unsupported clusterdata format version: %d", cd.FormatVersion)
	}
	if err = cd.Cluster.Spec.Validate(); err != nil {
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return fmt.Errorf("clusterdata validation failed: %v", err)
	}

	proxy := cd.Proxy
	if proxy == nil {
		log.Info("no proxy object available, closing connections to master")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(c.e, proxy.UID, proxy.Generation, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
			log.Error("failed to update proxyInfo", zap.Error(err))
		}
		return nil
	}

	db, ok := cd.DBs[proxy.Spec.MasterDBUID]
	if !ok {
		log.Info("no db object available, closing connections to master", zap.String("db", proxy.Spec.MasterDBUID))
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(c.e, proxy.UID, proxy.Generation, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
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
	// ignore errors on setting proxy info
	if err = c.SetProxyInfo(c.e, proxy.UID, proxy.Generation, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
		log.Error("failed to update proxyInfo", zap.Error(err))
	}

	c.sendPollonConfData(pollon.ConfData{DestAddr: addr})

	return nil
}

func (c *ClusterChecker) TimeoutChecker(checkOkCh chan struct{}) error {
	timeoutTimer := time.NewTimer(cluster.DefaultProxyTimeoutInterval)

	for true {
		select {
		case <-timeoutTimer.C:
			log.Info("check timeout timer fired")
			// if the check timeouts close all connections and stop listening
			// (for example to avoid load balancers forward connections to us
			// since we aren't ready or in a bad state)
			c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
			if c.stopListening {
				c.stopPollonProxy()
			}

		case <-checkOkCh:
			log.Debug("check ok message received")

			// ignore if stop succeeded or not due to timer already expired
			timeoutTimer.Stop()
			timeoutTimer = time.NewTimer(cluster.DefaultProxyTimeoutInterval)
		}
	}
	return nil
}

func (c *ClusterChecker) Start() error {
	checkOkCh := make(chan struct{})
	checkCh := make(chan error)
	timerCh := time.NewTimer(0).C

	go c.TimeoutChecker(checkOkCh)

	for true {
		select {
		case <-timerCh:
			go func() {
				checkCh <- c.Check()
			}()
		case err := <-checkCh:
			if err != nil {
				// don't report check ok since it returned an error
				log.Debug("check function error", zap.Error(err))
			} else {
				// report that check was ok
				checkOkCh <- struct{}{}
			}
			timerCh = time.NewTimer(cluster.DefaultProxyCheckInterval).C
		case err := <-c.endPollonProxyCh:
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
		die("cluster name required")
	}
	if cfg.storeBackend == "" {
		die("store backend type required")
	}

	uid := common.UID()
	log.Info("proxy uid", zap.String("uid", uid))

	clusterChecker, err := NewClusterChecker(uid, cfg)
	if err != nil {
		die("cannot create cluster checker: %v", err)
	}
	if err = clusterChecker.Start(); err != nil {
		die("cluster checker ended with error: %v", err)
	}
}
