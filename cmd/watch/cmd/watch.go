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

	"github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/pollon"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var log = slog.S()

var CmdWatch = &cobra.Command{
	Use:     "stolon-watch",
	Run:     watch,
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
	cmd.AddCommonFlags(CmdWatch, &cfg.CommonConfig)

	CmdWatch.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
	CmdWatch.PersistentFlags().IntVar(&cfg.keepAliveIdle, "tcp-keepalive-idle", 0, "set tcp keepalive idle (seconds)")
	CmdWatch.PersistentFlags().IntVar(&cfg.keepAliveCount, "tcp-keepalive-count", 0, "set tcp keepalive probe count number")
	CmdWatch.PersistentFlags().IntVar(&cfg.keepAliveInterval, "tcp-keepalive-interval", 0, "set tcp keepalive interval (seconds)")

	CmdWatch.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead")
}

type ClusterChecker struct {
	uid string
	e   store.Store

	pollonMutex sync.Mutex
}

func NewClusterChecker(uid string, cfg config) (*ClusterChecker, error) {
	e, err := cmd.NewStore(&cfg.CommonConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	return &ClusterChecker{
		uid: uid,
		e:   e,
	}, nil
}

// Check reads the cluster data
func (c *ClusterChecker) Check() error {
	cd, _, err := c.e.GetClusterData(context.TODO())
	if err != nil {
		return fmt.Errorf("cannot get cluster data: %v", err)
	}

	log.Debugf("cd dump: %s", spew.Sdump(cd))
	if cd == nil {
		log.Infow("no clusterdata available")
		return nil
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		return fmt.Errorf("unsupported clusterdata format version: %d", cd.FormatVersion)
	}
	if err = cd.Cluster.Spec.Validate(); err != nil {
		return fmt.Errorf("clusterdata validation failed: %v", err)
	}

	proxy := cd.Proxy
	if proxy == nil {
		log.Infow("no proxy object available")
		return nil
	}

	db, ok := cd.DBs[proxy.Spec.MasterDBUID]
	if !ok {
		log.Infow("no db object available, ", "db", proxy.Spec.MasterDBUID)
		return nil
	}

	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(db.Status.ListenAddress, db.Status.Port))
	if err != nil {
		log.Errorw("cannot resolve db address", zap.Error(err))
		return nil
	}
	log.Infow("master address", "address", addr)

	return nil
}

func (c *ClusterChecker) TimeoutChecker(checkOkCh chan struct{}) error {
	timeoutTimer := time.NewTimer(cluster.DefaultProxyTimeoutInterval)

	for true {
		select {
		case <-timeoutTimer.C:
			log.Infow("check timeout timer fired")
			// TODO

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
			timerCh = time.NewTimer(cluster.DefaultProxyCheckInterval).C
		}
	}
}

func Execute() {
	flagutil.SetFlagsFromEnv(CmdWatch.PersistentFlags(), "STWATCH")

	CmdWatch.Execute()
}

func watch(c *cobra.Command, args []string) {
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
		log.Fatalf("tcp keepalive count value must be greater or equal to 0")
	}
	if cfg.keepAliveInterval < 0 {
		log.Fatalf("tcp keepalive interval value must be greater or equal to 0")
	}

	uid := common.UID()
	log.Infow("watch uid", "uid", uid)

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
