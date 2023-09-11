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

package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/common"
	"github.com/sorintlab/stolon/internal/flagutil"
	slog "github.com/sorintlab/stolon/internal/log"
	"github.com/sorintlab/stolon/internal/store"
	"github.com/sorintlab/stolon/internal/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/pollon"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var log = slog.S()

var CmdProxy = &cobra.Command{
	Use:     "stolon-proxy",
	Run:     proxy,
	Version: cmd.Version,
}

type config struct {
	cmd.CommonConfig

	listenAddress       string
	port                string
	stopListening       bool
	replicaMode         bool
	replicaModeFallBack bool
	loadBalancingType   string
	debug               bool
	logPollon           bool

	keepAliveIdle     int
	keepAliveCount    int
	keepAliveInterval int
}

var cfg config

func init() {
	cmd.AddCommonFlags(CmdProxy, &cfg.CommonConfig)

	CmdProxy.PersistentFlags().StringVar(&cfg.listenAddress, "listen-address", "127.0.0.1", "proxy listening address")
	CmdProxy.PersistentFlags().StringVar(&cfg.port, "port", "5432", "proxy listening port")
	CmdProxy.PersistentFlags().BoolVar(&cfg.stopListening, "stop-listening", true, "stop listening on store error")
	CmdProxy.PersistentFlags().BoolVar(&cfg.replicaMode, "replica-mode", false, "proxy to replicas")
	CmdProxy.PersistentFlags().BoolVar(&cfg.replicaModeFallBack, "replica-mode-fallback", false, "Fallback to master when no lives replicas")
	CmdProxy.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
	CmdProxy.PersistentFlags().BoolVar(&cfg.logPollon, "log-pollon", false, "enable pollon logging")
	CmdProxy.PersistentFlags().IntVar(&cfg.keepAliveIdle, "tcp-keepalive-idle", 0, "set tcp keepalive idle (seconds)")
	CmdProxy.PersistentFlags().IntVar(&cfg.keepAliveCount, "tcp-keepalive-count", 0, "set tcp keepalive probe count number")
	CmdProxy.PersistentFlags().IntVar(&cfg.keepAliveInterval, "tcp-keepalive-interval", 0, "set tcp keepalive interval (seconds)")
	CmdProxy.PersistentFlags().StringVar(&cfg.loadBalancingType, "load-balancing-type", "random", "proxy to replicas LB Type")

	if err := CmdProxy.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead"); err != nil {
		log.Fatal(err)
	}
}

type ClusterChecker struct {
	uid           string
	listenAddress string
	port          string

	stopListening bool

	listener         *net.TCPListener
	pp               *pollon.Proxy
	e                store.Store
	endPollonProxyCh chan error

	pollonMutex sync.Mutex

	proxyCheckInterval time.Duration
	proxyTimeout       time.Duration
	configMutex        sync.Mutex
}

func NewClusterChecker(uid string, cfg config) (*ClusterChecker, error) {
	e, err := cmd.NewStore(&cfg.CommonConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	return &ClusterChecker{
		uid:              uid,
		listenAddress:    cfg.listenAddress,
		port:             cfg.port,
		stopListening:    cfg.stopListening,
		e:                e,
		endPollonProxyCh: make(chan error),

		proxyCheckInterval: cluster.DefaultProxyCheckInterval,
		proxyTimeout:       cluster.DefaultProxyTimeout,
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
	if cfg.loadBalancingType == "leastqueue" {
		pp.SetLBType(pollon.LeastQueue)
	}

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

func (c *ClusterChecker) SetProxyInfo(e store.Store, generation int64, proxyTimeout time.Duration) error {
	proxyInfo := &cluster.ProxyInfo{
		InfoUID:      common.UID(),
		UID:          c.uid,
		Generation:   generation,
		ProxyTimeout: proxyTimeout,
	}
	log.Debugf("proxyInfo dump: %s", spew.Sdump(proxyInfo))

	if err := c.e.SetProxyInfo(context.TODO(), proxyInfo, 2*proxyTimeout); err != nil {
		return err
	}
	return nil
}

func tcpAddrToStr(addr []*net.TCPAddr) string {
	var result string
	for i, a := range addr {
		if i > 0 {
			result += " "
		}
		result += a.String()
	}
	return result
}
func GetProxyDBs(cd *cluster.ClusterData) []*net.TCPAddr {
	var result []*net.TCPAddr
	//Proxy to master
	if !cfg.replicaMode {
		proxy := cd.Proxy
		db, ok := cd.DBs[proxy.Spec.MasterDBUID]
		if !ok {
			return nil
		}
		addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(db.Status.ListenAddress, db.Status.Port))
		if err != nil {
			return nil
		}
		result = append(result, addr)
		return result
	}
	//Proxy to replicas
	var master, replicas []*net.TCPAddr
	for _, db := range cd.DBs {
		if !db.Status.Healthy {
			continue
		}
		if len(db.Status.ListenAddress) < 1 || len(db.Status.ListenAddress) < 1 {
			continue
		}
		if db.Spec.Role == common.RoleStandby {
			addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(db.Status.ListenAddress, db.Status.Port))
			if err != nil {
				continue
			}
			replicas = append(replicas, addr)
		}
		if db.Spec.Role == common.RoleMaster {
			addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(db.Status.ListenAddress, db.Status.Port))
			if err != nil {
				continue
			}
			master = append(master, addr)
		}
	}
	if len(replicas) < 1 && cfg.replicaModeFallBack {
		log.Errorw("No alive replicas FallBack to master")
		return master
	}
	return replicas
}

// Check reads the cluster data and applies the right pollon configuration.
func (c *ClusterChecker) Check() error {
	cd, _, err := c.e.GetClusterData(context.TODO())
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

	cdProxyCheckInterval := cd.Cluster.DefSpec().ProxyCheckInterval.Duration
	cdProxyTimeout := cd.Cluster.DefSpec().ProxyTimeout.Duration

	// use the greater between the current proxy timeout and the one defined in the cluster spec if they're different.
	// in this way we're updating our proxyInfo using a timeout that is greater or equal the current active timeout timer.
	c.configMutex.Lock()
	proxyTimeout := c.proxyTimeout
	if cdProxyTimeout > proxyTimeout {
		proxyTimeout = cdProxyTimeout
	}
	c.configMutex.Unlock()

	proxy := cd.Proxy
	if proxy == nil {
		log.Infow("no proxy object available, closing connections to master")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(c.e, cluster.NoGeneration, proxyTimeout); err != nil {
			log.Errorw("failed to update proxyInfo", zap.Error(err))
		} else {
			// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
			c.configMutex.Lock()
			c.proxyCheckInterval = cdProxyCheckInterval
			c.proxyTimeout = cdProxyTimeout
			c.configMutex.Unlock()
		}
		return nil
	}

	addr := GetProxyDBs(cd)
	if len(addr) < 1 {
		log.Infow("no db object available, closing connections")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(c.e, proxy.Generation, proxyTimeout); err != nil {
			log.Errorw("failed to update proxyInfo", zap.Error(err))
		} else {
			// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
			c.configMutex.Lock()
			c.proxyCheckInterval = cdProxyCheckInterval
			c.proxyTimeout = cdProxyTimeout
			c.configMutex.Unlock()
		}
		return nil
	}

	log.Infow("Proxy address", "address", tcpAddrToStr(addr))
	if err = c.SetProxyInfo(c.e, proxy.Generation, proxyTimeout); err != nil {
		// if we failed to update our proxy info when a master is defined we
		// cannot ignore this error since the sentinel won't know that we exist
		// and are sending connections to a master so, when electing a new
		// master, it'll not wait for us to close connections to the old one.
		return fmt.Errorf("failed to update proxyInfo: %v", err)
	} else {
		// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
		c.configMutex.Lock()
		c.proxyCheckInterval = cdProxyCheckInterval
		c.proxyTimeout = cdProxyTimeout
		c.configMutex.Unlock()
	}

	// start proxing only if we are inside enabledProxies, this ensures that the
	// sentinel has read our proxyinfo and knows we are alive
	if util.StringInSlice(proxy.Spec.EnabledProxies, c.uid) {
		log.Infow("proxying to address", "address", tcpAddrToStr(addr))
		c.sendPollonConfData(pollon.ConfData{DestAddr: addr})
	} else {
		log.Infow("not proxying to address since we aren't in the enabled proxies list", "address", addr)
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
	}
	return nil
}

func (c *ClusterChecker) TimeoutChecker(checkOkCh chan struct{}) {
	c.configMutex.Lock()
	timeoutTimer := time.NewTimer(c.proxyTimeout)
	c.configMutex.Unlock()

	for {
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

			c.configMutex.Lock()
			timeoutTimer = time.NewTimer(c.proxyTimeout)
			c.configMutex.Unlock()
		}
	}
}

func (c *ClusterChecker) Start() error {
	checkOkCh := make(chan struct{})
	checkCh := make(chan error)
	timerCh := time.NewTimer(0).C

	// TODO(sgotti) TimeoutCecker is needed to forcefully close connection also
	// if the Check method is blocked somewhere.
	// The idomatic/cleaner solution will be to use a context instead of this
	// TimeoutChecker but we have to change the libkv stores to support contexts.
	go c.TimeoutChecker(checkOkCh)

	for {
		select {
		case <-timerCh:
			go func() {
				checkCh <- c.Check()
			}()
		case err := <-checkCh:
			if err != nil {
				// don't report check ok since it returned an error
				log.Infow("check function error", zap.Error(err))
			} else {
				// report that check was ok
				checkOkCh <- struct{}{}
			}
			c.configMutex.Lock()
			timerCh = time.NewTimer(c.proxyCheckInterval).C
			c.configMutex.Unlock()

		case err := <-c.endPollonProxyCh:
			if err != nil {
				return fmt.Errorf("proxy error: %v", err)
			}
		}
	}
}

func Execute() {
	if err := flagutil.SetFlagsFromEnv(CmdProxy.PersistentFlags(), "STPROXY"); err != nil {
		log.Fatal(err)
	}

	if err := CmdProxy.Execute(); err != nil {
		log.Fatal(err)
	}
}

func proxy(c *cobra.Command, args []string) {
	switch cfg.LogLevel {
	case "error":
		slog.SetLevel(zap.ErrorLevel)
	case "warn":
		slog.SetLevel(zap.WarnLevel)
	case "info":
		slog.SetLevel(zap.InfoLevel)
	case "debug":
		slog.SetLevel(zap.DebugLevel)
	default:
		log.Fatalf("invalid log level: %v", cfg.LogLevel)
	}
	if cfg.debug {
		slog.SetDebug()
	}
	if cmd.IsColorLoggerEnable(c, &cfg.CommonConfig) {
		log = slog.SColor()
	}
	if slog.IsDebug() || cfg.logPollon {
		if cmd.IsColorLoggerEnable(c, &cfg.CommonConfig) {
			stdlog := slog.StdLogColor()
			pollon.SetLogger(stdlog)
		} else {
			stdlog := slog.StdLog()
			pollon.SetLogger(stdlog)
		}
	}

	if err := cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		log.Fatalf(err.Error())
	}

	cmd.SetMetrics(&cfg.CommonConfig, "proxy")

	if cfg.keepAliveIdle < 0 {
		log.Fatalf("tcp keepalive idle value must be greater or equal to 0")
	}
	if cfg.keepAliveCount < 0 {
		log.Fatalf("tcp keepalive count value must be greater or equal to 0")
	}
	if cfg.keepAliveInterval < 0 {
		log.Fatalf("tcp keepalive interval value must be greater or equal to 0")
	}

	uid := common.UID()
	log.Infow("proxy uid", "uid", uid)

	if cfg.MetricsListenAddress != "" {
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			err := http.ListenAndServe(cfg.MetricsListenAddress, nil)
			if err != nil {
				log.Fatalf("metrics http server error", zap.Error(err))
			}
		}()
	}

	clusterChecker, err := NewClusterChecker(uid, cfg)
	if err != nil {
		log.Fatalf("cannot create cluster checker: %v", err)
	}
	if err = clusterChecker.Start(); err != nil {
		log.Fatalf("cluster checker ended with error: %v", err)
	}
}
