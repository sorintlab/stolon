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
	"sync"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/flagutil"
	slog "github.com/sorintlab/stolon/pkg/log"
	"github.com/sorintlab/stolon/pkg/store"
	"github.com/sorintlab/stolon/pkg/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/pollon"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var log = slog.S()

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
	logLevel           string
	debug              bool

	keepAliveIdle     int
	keepAliveCount    int
	keepAliveInterval int
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
	cmdProxy.PersistentFlags().StringVar(&cfg.logLevel, "log-level", "info", "debug, info (default), warn or error")
	cmdProxy.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
	cmdProxy.PersistentFlags().IntVar(&cfg.keepAliveIdle, "tcp-keepalive-idle", 0, "set tcp keepalive idle (seconds)")
	cmdProxy.PersistentFlags().IntVar(&cfg.keepAliveCount, "tcp-keepalive-count", 0, "set tcp keepalive probe count number")
	cmdProxy.PersistentFlags().IntVar(&cfg.keepAliveInterval, "tcp-keepalive-interval", 0, "set tcp keepalive interval (seconds)")

	cmdProxy.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead")
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

	pollonMutex sync.Mutex
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
	c.pollonMutex.Lock()
	defer c.pollonMutex.Unlock()
	if c.pp != nil {
		return nil
	}

	log.Infow("Starting proxying")
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
	pp.SetKeepAlive(true)
	pp.SetKeepAliveIdle(time.Duration(cfg.keepAliveIdle) * time.Second)
	pp.SetKeepAliveCount(cfg.keepAliveCount)
	pp.SetKeepAliveInterval(time.Duration(cfg.keepAliveInterval) * time.Second)

	c.pp = pp
	c.listener = listener

	go func() {
		c.endPollonProxyCh <- c.pp.Start()
	}()

	return nil
}

func (c *ClusterChecker) stopPollonProxy() {
	c.pollonMutex.Lock()
	defer c.pollonMutex.Unlock()
	if c.pp != nil {
		log.Infow("Stopping listening")
		c.pp.Stop()
		c.pp = nil
		c.listener.Close()
		c.listener = nil
	}
}

func (c *ClusterChecker) sendPollonConfData(confData pollon.ConfData) {
	c.pollonMutex.Lock()
	defer c.pollonMutex.Unlock()
	if c.pp != nil {
		c.pp.C <- confData
	}
}

func (c *ClusterChecker) SetProxyInfo(e *store.StoreManager, generation int64, ttl time.Duration) error {
	proxyInfo := &cluster.ProxyInfo{
		UID:        c.uid,
		Generation: generation,
	}
	log.Debugf("proxyInfo dump: %s", spew.Sdump(proxyInfo))

	if err := c.e.SetProxyInfo(proxyInfo, ttl); err != nil {
		return err
	}
	return nil
}

// Check reads the cluster data and applies the right pollon configuration.
func (c *ClusterChecker) Check() error {
	cd, _, err := c.e.GetClusterData()
	if err != nil {
		return fmt.Errorf("cannot get cluster data: %v", err)
	}

	// Start pollon if not active
	if err = c.startPollonProxy(); err != nil {
		return fmt.Errorf("failed to start proxy: %v", err)
	}

	log.Debugf("cd dump: %s", spew.Sdump(cd))
	if cd == nil {
		log.Infow("no clusterdata available, closing connections to master")
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
		log.Infow("no proxy object available, closing connections to master")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(c.e, cluster.NoGeneration, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
			log.Errorw("failed to update proxyInfo", zap.Error(err))
		}
		return nil
	}

	db, ok := cd.DBs[proxy.Spec.MasterDBUID]
	if !ok {
		log.Infow("no db object available, closing connections to master", "db", proxy.Spec.MasterDBUID)
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(c.e, proxy.Generation, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
			log.Errorw("failed to update proxyInfo", zap.Error(err))
		}
		return nil
	}

	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(db.Status.ListenAddress, db.Status.Port))
	if err != nil {
		log.Errorw("cannot resolve db address", zap.Error(err))
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return nil
	}
	log.Infow("master address", "address", addr)
	if err = c.SetProxyInfo(c.e, proxy.Generation, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
		// if we failed to update our proxy info when a master is defined we
		// cannot ignore this error since the sentinel won't know that we exist
		// and are sending connections to a master so, when electing a new
		// master, it'll not wait for us to close connections to the old one.
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		log.Errorw("failed to update proxyInfo", zap.Error(err))
		return nil
	}

	// start proxing only if we are inside enabledProxies, this ensures that the
	// sentinel has read our proxyinfo and knows we are alive
	if util.StringInSlice(proxy.Spec.EnabledProxies, c.uid) {
		log.Infow("proxying to master address", "address", addr)
		c.sendPollonConfData(pollon.ConfData{DestAddr: addr})
	} else {
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
	}

	return nil
}

func (c *ClusterChecker) TimeoutChecker(checkOkCh chan struct{}) error {
	timeoutTimer := time.NewTimer(cluster.DefaultProxyTimeoutInterval)

	for true {
		select {
		case <-timeoutTimer.C:
			log.Infow("check timeout timer fired")
			// if the check timeouts close all connections and stop listening
			// (for example to avoid load balancers forward connections to us
			// since we aren't ready or in a bad state)
			c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
			if c.stopListening {
				c.stopPollonProxy()
			}

		case <-checkOkCh:
			log.Debugw("check ok message received")

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
				log.Debugw("check function error", zap.Error(err))
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
	switch cfg.logLevel {
	case "error":
		slog.SetLevel(zap.ErrorLevel)
	case "warn":
		slog.SetLevel(zap.WarnLevel)
	case "info":
		slog.SetLevel(zap.InfoLevel)
	case "debug":
		slog.SetLevel(zap.DebugLevel)
	default:
		die("invalid log level: %v", cfg.logLevel)
	}
	if cfg.debug {
		slog.SetDebug()
	}
	if slog.IsDebug() {
		stdlog := slog.StdLog()
		pollon.SetLogger(stdlog)
	}

	if cfg.clusterName == "" {
		die("cluster name required")
	}
	if cfg.storeBackend == "" {
		die("store backend type required")
	}
	if cfg.keepAliveIdle < 0 {
		die("tcp keepalive idle value must be greater or equal to 0")
	}
	if cfg.keepAliveCount < 0 {
		die("tcp keepalive idle value must be greater or equal to 0")
	}
	if cfg.keepAliveInterval < 0 {
		die("tcp keepalive idle value must be greater or equal to 0")
	}

	uid := common.UID()
	log.Infow("proxy uid", "uid", uid)

	clusterChecker, err := NewClusterChecker(uid, cfg)
	if err != nil {
		die("cannot create cluster checker: %v", err)
	}
	if err = clusterChecker.Start(); err != nil {
		die("cluster checker ended with error: %v", err)
	}
}
