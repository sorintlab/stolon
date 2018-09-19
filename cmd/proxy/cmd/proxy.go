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
	"os"
	"os/signal"
	"sync"
	"syscall"
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

	listenAddress string
	port          string
	stopListening bool
	debug         bool

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
	CmdProxy.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
	CmdProxy.PersistentFlags().IntVar(&cfg.keepAliveIdle, "tcp-keepalive-idle", 0, "set tcp keepalive idle (seconds)")
	CmdProxy.PersistentFlags().IntVar(&cfg.keepAliveCount, "tcp-keepalive-count", 0, "set tcp keepalive probe count number")
	CmdProxy.PersistentFlags().IntVar(&cfg.keepAliveInterval, "tcp-keepalive-interval", 0, "set tcp keepalive interval (seconds)")

	CmdProxy.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead")
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

func (c *ClusterChecker) updateDestAddress(destAddr *net.TCPAddr) {
	c.pollonMutex.Lock()
	defer c.pollonMutex.Unlock()
	if c.pp != nil {
		c.pp.C <- pollon.ConfData{DestAddr: destAddr}
	}
}

func (c *ClusterChecker) setProxyInfo(ctx context.Context, e store.Store, generation int64, ttl time.Duration) error {
	proxyInfo := &cluster.ProxyInfo{
		InfoUID:    common.UID(),
		UID:        c.uid,
		Generation: generation,
	}
	log.Debugf("proxyInfo dump: %s", spew.Sdump(proxyInfo))

	if err := c.e.SetProxyInfo(ctx, proxyInfo, ttl); err != nil {
		return err
	}
	return nil
}

// check reads the cluster data and applies the right pollon configuration.
func (c *ClusterChecker) check(ctx context.Context) error {
	cd, _, err := c.e.GetClusterData(ctx)
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
		c.updateDestAddress(nil)
		return nil
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		c.updateDestAddress(nil)
		return fmt.Errorf("unsupported clusterdata format version: %d", cd.FormatVersion)
	}
	if err = cd.Cluster.Spec.Validate(); err != nil {
		c.updateDestAddress(nil)
		return fmt.Errorf("clusterdata validation failed: %v", err)
	}

	proxy := cd.Proxy
	if proxy == nil {
		log.Infow("no proxy object available, closing connections to master")
		c.updateDestAddress(nil)
		// ignore errors on setting proxy info
		if err = c.setProxyInfo(ctx, c.e, cluster.NoGeneration, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
			log.Errorw("failed to update proxyInfo", zap.Error(err))
		}
		return nil
	}

	db, ok := cd.DBs[proxy.Spec.MasterDBUID]
	if !ok {
		log.Infow("no db object available, closing connections to master", "db", proxy.Spec.MasterDBUID)
		c.updateDestAddress(nil)
		// ignore errors on setting proxy info
		if err = c.setProxyInfo(ctx, c.e, proxy.Generation, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
			log.Errorw("failed to update proxyInfo", zap.Error(err))
		}
		return nil
	}

	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, db.Status.ListenAddress)
	if err != nil {
		log.Errorw("cannot resolve db address", zap.Error(err))
		c.updateDestAddress(nil)
		return nil
	}
	if len(ipAddrs) == 0 {
		log.Errorw("cannot resolve db address: no addresses returned from lookup")
		c.updateDestAddress(nil)
		return nil
	}
	ipAddr := ipAddrs[0]
	port, err := net.DefaultResolver.LookupPort(ctx, "tcp", db.Status.Port)
	if err != nil {
		log.Errorw("cannot resolve port", zap.Error(err))
		c.updateDestAddress(nil)
		return nil
	}

	tcpAddr := &net.TCPAddr{IP: ipAddr.IP, Port: port, Zone: ipAddr.Zone}
	log.Infow("master address", "address", tcpAddr)

	if err = c.setProxyInfo(ctx, c.e, proxy.Generation, 2*cluster.DefaultProxyTimeoutInterval); err != nil {
		// if we failed to update our proxy info when a master is defined we
		// cannot ignore this error since the sentinel won't know that we exist
		// and are sending connections to a master so, when electing a new
		// master, it'll not wait for us to close connections to the old one.
		return fmt.Errorf("failed to update proxyInfo: %v", err)
	}

	// start proxing only if we are inside enabledProxies, this ensures that the
	// sentinel has read our proxyinfo and knows we are alive
	if !util.StringInSlice(proxy.Spec.EnabledProxies, c.uid) {
		log.Infow("not proxying to master address since we aren't in the enabled proxies list", "address", tcpAddr)
		c.updateDestAddress(nil)
		return nil
	}

	// before updating the pollon address, check that the context isn't timed
	// out, usually if the context is timeout out one of the above calls will
	// return an error but libkv stores doesn't handle contexts so we should
	// check here.
	select {
	default:
	case <-ctx.Done():
		log.Infow("not updating proxy address since context is done: %v", ctx.Err())
		return nil
	}

	log.Infow("proxying to master address", "address", tcpAddr)
	c.updateDestAddress(tcpAddr)

	return nil
}

// timeoutChecker will forcefully close connections when the context times
// out.
func (c *ClusterChecker) timeoutChecker(ctx context.Context) {
	<-ctx.Done()
	if ctx.Err() == context.DeadlineExceeded {
		log.Infow("check timeout timer fired")
		// if the check timeouts close all connections and stop listening
		// (for example to avoid load balancers forward connections to us
		// since we aren't ready or in a bad state)
		c.updateDestAddress(nil)
		if c.stopListening {
			c.stopPollonProxy()
		}
	}
}

// checkLoop executes at predefined intervals the Check function. It'll force
// close connections when a check function continuosly fails for more than a
// timeout.
func (c *ClusterChecker) checkLoop(pctx context.Context) error {
	checkCh := make(chan error)
	timerCh := time.NewTimer(0).C

	ctx, cancel := context.WithTimeout(pctx, cluster.DefaultProxyTimeoutInterval)

	for {
		select {
		case <-pctx.Done():
			cancel()
			return nil
		case <-timerCh:
			// start a new context if it's already done, this happens when the
			// context is timed out or cancelled.
			select {
			default:
			case <-ctx.Done():
				ctx, cancel = context.WithTimeout(pctx, cluster.DefaultProxyTimeoutInterval)
			}

			go func() {
				checkCh <- c.check(ctx)
			}()
		case err := <-checkCh:
			if err != nil {
				// if the check function returned an error then don't stop the
				// context so if it times out the TimeoutChecker will close
				// connections or it could be cancelled if the next check
				// succeeds before the timeout
				log.Infow("check function error", zap.Error(err))
			} else {
				// check was ok, so cancel the context and start a new one with a TimeoutChecker
				cancel()
				ctx, cancel = context.WithTimeout(pctx, cluster.DefaultProxyTimeoutInterval)
				go c.timeoutChecker(ctx)
			}
			timerCh = time.NewTimer(cluster.DefaultProxyCheckInterval).C
		case err := <-c.endPollonProxyCh:
			if err != nil {
				cancel()
				return fmt.Errorf("proxy error: %v", err)
			}
		}
	}
}

func sigHandler(sigs chan os.Signal, cancel context.CancelFunc) {
	s := <-sigs
	log.Debugw("got signal", "signal", s)
	cancel()
}

func Execute() {
	flagutil.SetFlagsFromEnv(CmdProxy.PersistentFlags(), "STPROXY")

	CmdProxy.Execute()
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
	if slog.IsDebug() {
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

	if cfg.keepAliveIdle < 0 {
		log.Fatalf("tcp keepalive idle value must be greater or equal to 0")
	}
	if cfg.keepAliveCount < 0 {
		log.Fatalf("tcp keepalive idle value must be greater or equal to 0")
	}
	if cfg.keepAliveInterval < 0 {
		log.Fatalf("tcp keepalive idle value must be greater or equal to 0")
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

	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go sigHandler(sigs, cancel)

	clusterChecker, err := NewClusterChecker(uid, cfg)
	if err != nil {
		log.Fatalf("cannot create cluster checker: %v", err)
	}
	if err = clusterChecker.checkLoop(ctx); err != nil {
		log.Fatalf("cluster checker ended with error: %v", err)
	}
}
