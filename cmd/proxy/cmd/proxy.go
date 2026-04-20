// Copyright 2026 PgVillage
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

// Package cmd holds all CLI code for the keeper
package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	cluster "github.com/pgvillage-tools/stolon/api/v1"
	"github.com/pgvillage-tools/stolon/cmd"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/flagutil"
	"github.com/pgvillage-tools/stolon/internal/logging"
	"github.com/pgvillage-tools/stolon/internal/store"
	"github.com/pgvillage-tools/stolon/internal/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/pollon"
	"github.com/spf13/cobra"
)

// CmdProxy is the cobra command which defines running stolon-proxy
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

	CmdProxy.PersistentFlags().StringVar(
		&cfg.listenAddress,
		"listen-address",
		"127.0.0.1",
		"proxy listening address")
	CmdProxy.PersistentFlags().StringVar(
		&cfg.port,
		"port",
		"5432",
		"proxy listening port")
	CmdProxy.PersistentFlags().BoolVar(
		&cfg.stopListening,
		"stop-listening",
		true,
		"stop listening on store error")
	CmdProxy.PersistentFlags().BoolVar(
		&cfg.debug,
		"debug",
		false,
		"enable debug logging")
	CmdProxy.PersistentFlags().IntVar(
		&cfg.keepAliveIdle,
		"tcp-keepalive-idle",
		0,
		"set tcp keepalive idle (seconds)")
	CmdProxy.PersistentFlags().IntVar(
		&cfg.keepAliveCount,
		"tcp-keepalive-count",
		0,
		"set tcp keepalive probe count number")
	CmdProxy.PersistentFlags().IntVar(
		&cfg.keepAliveInterval,
		"tcp-keepalive-interval",
		0,
		"set tcp keepalive interval (seconds)")

	if err := CmdProxy.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead"); err != nil {
		_, logger := logging.GetLogComponent(context.Background(), logging.ProxyComponent)
		logger.Fatal().AnErr("err", err).Msg("")
	}
}

// ClusterChecker is struct containing information for checking the cluster
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

// NewClusterChecker is a function which creates a new clusterchecker
func NewClusterChecker(ctx context.Context, uid string, cfg config) (*ClusterChecker, error) {
	e, err := cmd.NewStore(ctx, &cfg.CommonConfig)
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

func (c *ClusterChecker) startPollonProxy(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	c.pollonMutex.Lock()
	defer c.pollonMutex.Unlock()
	if c.pp != nil {
		return nil
	}

	logger.Info().Msg("Starting proxying")
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

func (c *ClusterChecker) stopPollonProxy(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	c.pollonMutex.Lock()
	defer c.pollonMutex.Unlock()
	if c.pp != nil {
		logger.Info().Msg("Stopping listening")
		c.pp.Stop()
		c.pp = nil
		if err := c.listener.Close(); err != nil {
			logger.Fatal().AnErr("err", err).Msg("Stopping listening")
		}
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

// SetProxyInfo is function which sets the proxy-info
func (c *ClusterChecker) SetProxyInfo(ctx context.Context, _ store.Store, generation int64,
	proxyTimeout time.Duration) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	proxyInfo := &cluster.ProxyInfo{
		InfoUID:      common.UID(),
		UID:          c.uid,
		Generation:   generation,
		ProxyTimeout: proxyTimeout,
	}
	logger.Debug().Str("proxyInfo dump", spew.Sdump(proxyInfo)).Msg("")

	return c.e.SetProxyInfo(ctx, proxyInfo, 2*proxyTimeout)
}

// Check reads the cluster data and applies the right pollon configuration.
func (c *ClusterChecker) Check(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	cd, _, err := c.e.GetClusterData(ctx)
	if err != nil {
		return fmt.Errorf("cannot get cluster data: %v", err)
	}

	// Start pollon if not active
	if err = c.startPollonProxy(ctx); err != nil {
		return fmt.Errorf("failed to start proxy: %v", err)
	}

	logger.Debug().Str("cd dump", spew.Sdump(cd)).Msg("")
	if cd == nil {
		logger.Info().Msg("no clusterdata available, closing connections to master")
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
	// in this way we're updating our proxyInfo using a timeout that is greater
	// or equal the current active timeout timer.
	c.configMutex.Lock()
	proxyTimeout := c.proxyTimeout
	if cdProxyTimeout > proxyTimeout {
		proxyTimeout = cdProxyTimeout
	}
	c.configMutex.Unlock()

	proxy := cd.Proxy
	if proxy == nil {
		logger.Info().Msg("no proxy object available, closing connections to master")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(ctx, c.e, cluster.NoGeneration, proxyTimeout); err != nil {
			logger.Error().AnErr("err", err).Msg("failed to update proxyInfo")
		} else {
			// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
			c.configMutex.Lock()
			c.proxyCheckInterval = cdProxyCheckInterval
			c.proxyTimeout = cdProxyTimeout
			c.configMutex.Unlock()
		}
		return nil
	}

	db, ok := cd.DBs[proxy.Spec.MasterDBUID]
	if !ok {
		logger.Info().Str("db", proxy.Spec.MasterDBUID).Msg("no db object available, closing connections to master")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(ctx, c.e, proxy.Generation, proxyTimeout); err != nil {
			logger.Error().AnErr("err", err).Msg("failed to update proxyInfo")
		} else {
			// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
			c.configMutex.Lock()
			c.proxyCheckInterval = cdProxyCheckInterval
			c.proxyTimeout = cdProxyTimeout
			c.configMutex.Unlock()
		}
		return nil
	}

	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(db.Status.ListenAddress, db.Status.Port))
	if err != nil {
		logger.Error().AnErr("err", err).Msg("cannot resolve db address")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
		return nil
	}
	logger.Info().Any("address", addr).Msg("master address")
	if err = c.SetProxyInfo(ctx, c.e, proxy.Generation, proxyTimeout); err != nil {
		// if we failed to update our proxy info when a master is defined we
		// cannot ignore this error since the sentinel won't know that we exist
		// and are sending connections to a master so, when electing a new
		// master, it'll not wait for us to close connections to the old one.
		return fmt.Errorf("failed to update proxyInfo: %v", err)
	}
	// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
	c.configMutex.Lock()
	c.proxyCheckInterval = cdProxyCheckInterval
	c.proxyTimeout = cdProxyTimeout
	c.configMutex.Unlock()

	// start proxing only if we are inside enabledProxies, this ensures that the
	// sentinel has read our proxyinfo and knows we are alive
	if util.StringInSlice(proxy.Spec.EnabledProxies, c.uid) {
		logger.Info().Any("address", addr).Msg("proxying to master address")
		c.sendPollonConfData(pollon.ConfData{DestAddr: addr})
	} else {
		logger.Info().
			Any("address", addr).
			Msg("not proxying to master address since we aren't in the enabled proxies list")
		c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
	}

	return nil
}

// TimeoutChecker is a function that checks the timeouts
func (c *ClusterChecker) TimeoutChecker(ctx context.Context, checkOkCh chan struct{}) {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	c.configMutex.Lock()
	timeoutTimer := time.NewTimer(c.proxyTimeout)
	c.configMutex.Unlock()

	for {
		select {
		case <-timeoutTimer.C:
			logger.Info().Msg("check timeout timer fired")
			// if the check timeouts close all connections and stop listening
			// (for example to avoid load balancers forward connections to us
			// since we aren't ready or in a bad state)
			c.sendPollonConfData(pollon.ConfData{DestAddr: nil})
			if c.stopListening {
				c.stopPollonProxy(ctx)
			}

		case <-checkOkCh:
			logger.Debug().Msg("check ok message received")

			// ignore if stop succeeded or not due to timer already expired
			timeoutTimer.Stop()

			c.configMutex.Lock()
			timeoutTimer = time.NewTimer(c.proxyTimeout)
			c.configMutex.Unlock()
		}
	}
}

// Start is function that starts the procy
func (c *ClusterChecker) Start(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	checkOkCh := make(chan struct{})
	checkCh := make(chan error)
	timerCh := time.NewTimer(0).C

	// TODO(sgotti) TimeoutCecker is needed to forcefully close connection also
	// if the Check method is blocked somewhere.
	// The idomatic/cleaner solution will be to use a context instead of this
	// TimeoutChecker but we have to change the libkv stores to support contexts.
	go c.TimeoutChecker(ctx, checkOkCh)

	for {
		select {
		case <-timerCh:
			go func() {
				checkCh <- c.Check(ctx)
			}()
		case err := <-checkCh:
			if err != nil {
				// don't report check ok since it returned an error
				logger.Info().AnErr("err", err).Msg("check function error")
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

// Execute is the main executor of the proxy
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.ProxyComponent)
	if err := flagutil.SetFlagsFromEnv(CmdProxy.PersistentFlags(), "STPROXY"); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}

	if err := CmdProxy.Execute(); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}
}

func proxy(c *cobra.Command, _ []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	defer cancelFunc()
	logging.SetStaticLevel(cfg.LogLevel)
	if cfg.debug {
		logging.SetStaticLevel("debug")
	}
	if cmd.IsColorLoggerEnable(c, &cfg.CommonConfig) {
		logging.EnableColor()
	}

	if err := cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}

	cmd.SetMetrics(&cfg.CommonConfig, "proxy")

	if cfg.keepAliveIdle < 0 {
		logger.Fatal().Msgf("tcp keepalive idle value must be greater or equal to 0")
	}
	if cfg.keepAliveCount < 0 {
		logger.Fatal().Msgf("tcp keepalive count value must be greater or equal to 0")
	}
	if cfg.keepAliveInterval < 0 {
		logger.Fatal().Msgf("tcp keepalive interval value must be greater or equal to 0")
	}

	uid := common.UID()
	logger.Info().Str("uid", uid).Msg("proxy uid")

	if cfg.MetricsListenAddress != "" {
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			err := http.ListenAndServe(cfg.MetricsListenAddress, nil)
			if err != nil {
				logger.Fatal().AnErr("err", err).Msg("metrics http server error")
			}
		}()
	}

	clusterChecker, err := NewClusterChecker(ctx, uid, cfg)
	if err != nil {
		logger.Fatal().Msgf("cannot create cluster checker: %v", err)
	}
	if err = clusterChecker.Start(ctx); err != nil {
		logger.Fatal().Msgf("cluster checker ended with error: %v", err)
	}
}
