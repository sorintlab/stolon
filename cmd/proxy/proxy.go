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

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	etcdm "github.com/sorintlab/stolon/pkg/etcd"
	"github.com/sorintlab/stolon/pkg/flagutil"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/pkg/capnslog"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/sorintlab/pollon"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/spf13/cobra"
)

var log = capnslog.NewPackageLogger("github.com/sorintlab/stolon/cmd", "pgproxy")

func init() {
	capnslog.SetFormatter(capnslog.NewPrettyFormatter(os.Stderr, true))
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
}

var cmdProxy = &cobra.Command{
	Use: "stolon-proxy",
	Run: proxy,
}

type config struct {
	etcdEndpoints string
	clusterName   string
	listenAddress string
	port          string
	debug         bool
}

var cfg config

func init() {
	cmdProxy.PersistentFlags().StringVar(&cfg.etcdEndpoints, "etcd-endpoints", "http://127.0.0.1:4001,http://127.0.0.1:2379", "a comma-delimited list of etcd endpoints")
	cmdProxy.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdProxy.PersistentFlags().StringVar(&cfg.listenAddress, "listen-address", "127.0.0.1", "proxy listening address")
	cmdProxy.PersistentFlags().StringVar(&cfg.port, "port", "5432", "proxy listening port")
	cmdProxy.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
}

type ClusterChecker struct {
	e            *etcdm.EtcdManager
	prevMasterID string
}

func NewClusterChecker(cfg config) *ClusterChecker {
	clusterConfig := cluster.NewDefaultConfig()
	etcdPath := filepath.Join(common.EtcdBasePath, cfg.clusterName)
	e, err := etcdm.NewEtcdManager(cfg.etcdEndpoints, etcdPath, clusterConfig.RequestTimeout)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	return &ClusterChecker{e: e}
}

func (c *ClusterChecker) Check() (*net.TCPAddr, error) {
	pv, _, err := c.e.GetProxyView()
	if err != nil {
		log.Errorf("err: %v", err)
		return nil, err
	}
	log.Debugf("proxyview: %v", pv)
	if pv == nil {
		log.Infof("no proxyview available")
		return nil, nil
	}
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%s", pv.Host, pv.Port))
	if err != nil {
		log.Errorf("err: %v", err)
		return nil, err
	}
	log.Infof("addr: %v", addr)
	return addr, nil
}

func main() {
	flagutil.SetFlagsFromEnv(cmdProxy.PersistentFlags(), "STPROXY")

	cmdProxy.Execute()
}

func proxy(cmd *cobra.Command, args []string) {
	capnslog.SetGlobalLogLevel(capnslog.INFO)
	if cfg.debug {
		capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	}
	if cfg.clusterName == "" {
		log.Fatalf("cluster name required")
	}

	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])
	log.Infof("id: %s", id)

	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cfg.listenAddress, cfg.port))
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	clusterConfig := cluster.NewDefaultConfig()
	proxyConfig := &pollon.Config{
		ConfChecker:        NewClusterChecker(cfg),
		CheckInterval:      clusterConfig.SleepInterval,
		ExitOnCheckerError: false,
	}
	proxy, err := pollon.NewProxy(listener, proxyConfig)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	err = proxy.Start()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
