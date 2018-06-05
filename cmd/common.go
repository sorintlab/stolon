// Copyright 2017 Sorint.lab
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
	"fmt"
	"os"
	"path/filepath"

	"github.com/sorintlab/stolon/internal/common"
	"github.com/sorintlab/stolon/internal/store"
	"github.com/sorintlab/stolon/internal/util"
	"k8s.io/client-go/kubernetes"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type CommonConfig struct {
	IsStolonCtl bool

	StoreBackend         string
	StoreEndpoints       string
	StorePrefix          string
	StoreCertFile        string
	StoreKeyFile         string
	StoreCAFile          string
	StoreSkipTlsVerify   bool
	ClusterName          string
	MetricsListenAddress string
	LogColor             bool
	LogLevel             string
	Debug                bool
	KubeResourceKind     string
	KubeConfig           string
	KubeContext          string
	KubeNamespace        string
}

func AddCommonFlags(cmd *cobra.Command, cfg *CommonConfig) {
	cmd.PersistentFlags().StringVar(&cfg.ClusterName, "cluster-name", "", "cluster name")
	cmd.PersistentFlags().StringVar(&cfg.StoreBackend, "store-backend", "", "store backend type (etcdv2/etcd, etcdv3, consul or kubernetes)")
	cmd.PersistentFlags().StringVar(&cfg.StoreEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (use https scheme for tls communication) (defaults: http://127.0.0.1:2379 for etcd, http://127.0.0.1:8500 for consul)")
	cmd.PersistentFlags().StringVar(&cfg.StorePrefix, "store-prefix", common.StorePrefix, "the store base prefix")
	cmd.PersistentFlags().StringVar(&cfg.StoreCertFile, "store-cert-file", "", "certificate file for client identification to the store")
	cmd.PersistentFlags().StringVar(&cfg.StoreKeyFile, "store-key", "", "private key file for client identification to the store")
	cmd.PersistentFlags().BoolVar(&cfg.StoreSkipTlsVerify, "store-skip-tls-verify", false, "skip store certificate verification (insecure!!!)")
	cmd.PersistentFlags().StringVar(&cfg.StoreCAFile, "store-ca-file", "", "verify certificates of HTTPS-enabled store servers using this CA bundle")
	cmd.PersistentFlags().StringVar(&cfg.MetricsListenAddress, "metrics-listen-address", "", "metrics listen address i.e \"0.0.0.0:8080\" (disabled by default)")
	cmd.PersistentFlags().StringVar(&cfg.KubeResourceKind, "kube-resource-kind", "", `the k8s resource kind to be used to store stolon clusterdata and do sentinel leader election (only "configmap" is currently supported)`)

	if !cfg.IsStolonCtl {
		cmd.PersistentFlags().BoolVar(&cfg.LogColor, "log-color", false, "enable color in log output (default if attached to a terminal)")
		cmd.PersistentFlags().StringVar(&cfg.LogLevel, "log-level", "info", "debug, info (default), warn or error")
	}

	if cfg.IsStolonCtl {
		cmd.PersistentFlags().StringVar(&cfg.LogLevel, "log-level", "info", "debug, info (default), warn or error")
		cmd.PersistentFlags().StringVar(&cfg.KubeConfig, "kubeconfig", "", "path to kubeconfig file. Overrides $KUBECONFIG")
		cmd.PersistentFlags().StringVar(&cfg.KubeContext, "kube-context", "", "name of the kubeconfig context to use")
		cmd.PersistentFlags().StringVar(&cfg.KubeNamespace, "kube-namespace", "", "name of the kubernetes namespace to use")
	}
}

func CheckCommonConfig(cfg *CommonConfig) error {
	if cfg.ClusterName == "" {
		return fmt.Errorf("cluster name required")
	}
	if cfg.StoreBackend == "" {
		return fmt.Errorf("store backend type required")
	}

	switch cfg.StoreBackend {
	case "consul":
	case "etcd":
		// etcd is old alias for etcdv2
		cfg.StoreBackend = "etcdv2"
	case "etcdv2":
	case "etcdv3":
	case "kubernetes":
		if cfg.KubeResourceKind == "" {
			return fmt.Errorf("unspecified kubernetes resource kind")
		}
		if cfg.KubeResourceKind != "configmap" {
			return fmt.Errorf("wrong kubernetes resource kind: %q", cfg.KubeResourceKind)
		}
	default:
		return fmt.Errorf("Unknown store backend: %q", cfg.StoreBackend)
	}

	return nil
}

func IsColorLoggerEnable(cmd *cobra.Command, cfg *CommonConfig) bool {
	if cmd.PersistentFlags().Changed("log-color") {
		return cfg.LogColor
	} else {
		return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	}
}

func NewKVStore(cfg *CommonConfig) (store.KVStore, error) {
	return store.NewKVStore(store.Config{
		Backend:       store.Backend(cfg.StoreBackend),
		Endpoints:     cfg.StoreEndpoints,
		CertFile:      cfg.StoreCertFile,
		KeyFile:       cfg.StoreKeyFile,
		CAFile:        cfg.StoreCAFile,
		SkipTLSVerify: cfg.StoreSkipTlsVerify,
	})
}

func NewStore(cfg *CommonConfig) (store.Store, error) {
	var s store.Store

	switch cfg.StoreBackend {
	case "consul":
		fallthrough
	case "etcdv2":
		fallthrough
	case "etcdv3":
		storePath := filepath.Join(cfg.StorePrefix, cfg.ClusterName)

		kvstore, err := NewKVStore(cfg)
		if err != nil {
			return nil, fmt.Errorf("cannot create kv store: %v", err)
		}
		s = store.NewKVBackedStore(kvstore, storePath)
	case "kubernetes":
		kubeClientConfig := util.NewKubeClientConfig(cfg.KubeConfig, cfg.KubeContext, cfg.KubeNamespace)
		kubecfg, err := kubeClientConfig.ClientConfig()
		if err != nil {
			return nil, err
		}
		kubecli, err := kubernetes.NewForConfig(kubecfg)
		if err != nil {
			return nil, fmt.Errorf("cannot create kubernetes client: %v", err)
		}
		var podName string
		if !cfg.IsStolonCtl {
			podName, err = util.PodName()
			if err != nil {
				return nil, err
			}
		}
		namespace, _, err := kubeClientConfig.Namespace()
		if err != nil {
			return nil, err
		}
		s, err = store.NewKubeStore(kubecli, podName, namespace, cfg.ClusterName)
		if err != nil {
			return nil, fmt.Errorf("cannot create store: %v", err)
		}
	}

	return s, nil
}

func NewElection(cfg *CommonConfig, uid string) (store.Election, error) {
	var election store.Election

	switch cfg.StoreBackend {
	case "consul":
		fallthrough
	case "etcdv2":
		fallthrough
	case "etcdv3":
		storePath := filepath.Join(cfg.StorePrefix, cfg.ClusterName)

		kvstore, err := NewKVStore(cfg)
		if err != nil {
			return nil, fmt.Errorf("cannot create kv store: %v", err)
		}
		election = store.NewKVBackedElection(kvstore, filepath.Join(storePath, common.SentinelLeaderKey), uid)
	case "kubernetes":
		kubeClientConfig := util.NewKubeClientConfig(cfg.KubeConfig, cfg.KubeContext, cfg.KubeNamespace)
		kubecfg, err := kubeClientConfig.ClientConfig()
		if err != nil {
			return nil, err
		}
		kubecli, err := kubernetes.NewForConfig(kubecfg)
		if err != nil {
			return nil, fmt.Errorf("cannot create kubernetes client: %v", err)
		}
		var podName string
		if !cfg.IsStolonCtl {
			podName, err = util.PodName()
			if err != nil {
				return nil, err
			}
		}
		namespace, _, err := kubeClientConfig.Namespace()
		if err != nil {
			return nil, err
		}
		election, err = store.NewKubeElection(kubecli, podName, namespace, cfg.ClusterName, uid)
		if err != nil {
			return nil, err
		}
	}

	return election, nil
}
