// Copyright 2026 PgVillage
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

// Package cmd is a package which provides utilities that underlie the specific command
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mattn/go-isatty"
	cluster "github.com/pgvillage-tools/stolon/api/v1"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/store"
	"github.com/pgvillage-tools/stolon/internal/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
)

// CommonConfig is a struct specifying if certain objects are strings or booleans for example
type CommonConfig struct {
	IsStolonCtl bool

	StoreBackend         string
	StoreEndpoints       string
	StorePrefix          string
	StoreCertFile        string
	StoreKeyFile         string
	StoreCAFile          string
	StoreSkipTLSVerify   bool
	ClusterName          string
	MetricsListenAddress string
	LogColor             bool
	LogLevel             string
	Debug                bool
	KubeResourceKind     string
	KubeConfig           string
	KubeContext          string
	KubeNamespace        string
	StoreTimeout         time.Duration
}

// AddCommonFlags is a function that gives flags to commands
func AddCommonFlags(cmd *cobra.Command, cfg *CommonConfig) {
	cmd.PersistentFlags().StringVar(
		&cfg.ClusterName,
		"cluster-name",
		"",
		"cluster name")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreBackend,
		"store-backend",
		"",
		"store backend type (etcdv2/etcd, etcdv3, consul or kubernetes)")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreEndpoints,
		"store-endpoints",
		"",
		// revive:disable-next-line
		"a comma-delimited list of store endpoints (use https scheme for tls communication) (defaults: http://127.0.0.1:2379 for etcd, http://127.0.0.1:8500 for consul)")

	cmd.PersistentFlags().DurationVar(
		&cfg.StoreTimeout,
		"store-timeout",
		cluster.DefaultStoreTimeout,
		"store request timeout")

	cmd.PersistentFlags().StringVar(
		&cfg.StorePrefix,
		"store-prefix",
		common.StorePrefix,
		"the store base prefix")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreCertFile,
		"store-cert-file",
		"",
		"certificate file for client identification to the store")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreKeyFile,
		"store-key",
		"",
		"private key file for client identification to the store")

	cmd.PersistentFlags().BoolVar(
		&cfg.StoreSkipTLSVerify,
		"store-skip-tls-verify",
		false,
		"skip store certificate verification (insecure!!!)")

	cmd.PersistentFlags().StringVar(
		&cfg.StoreCAFile,
		"store-ca-file",
		"",
		"verify certificates of HTTPS-enabled store servers using this CA bundle")

	cmd.PersistentFlags().StringVar(
		&cfg.MetricsListenAddress,
		"metrics-listen-address",
		"",
		"metrics listen address i.e \"0.0.0.0:8080\" (disabled by default)")

	cmd.PersistentFlags().StringVar(
		&cfg.KubeResourceKind,
		"kube-resource-kind",
		"",
		// revive:disable-next-line
		`the k8s resource kind to be used to store stolon clusterdata and do sentinel leader election (only "configmap" is currently supported)`)

	if !cfg.IsStolonCtl {
		cmd.PersistentFlags().BoolVar(
			&cfg.LogColor,
			"log-color",
			false,
			"enable color in log output (default if attached to a terminal)")
		cmd.PersistentFlags().StringVar(
			&cfg.LogLevel,
			"log-level",
			"info",
			"debug, info (default), warn or error")
	}

	if cfg.IsStolonCtl {
		cmd.PersistentFlags().StringVar(
			&cfg.LogLevel,
			"log-level",
			"info",
			"debug, info (default), warn or error")
		cmd.PersistentFlags().StringVar(
			&cfg.KubeConfig,
			"kubeconfig",
			"",
			"path to kubeconfig file. Overrides $KUBECONFIG")
		cmd.PersistentFlags().StringVar(
			&cfg.KubeContext,
			"kube-context",
			"",
			"name of the kubeconfig context to use")
		cmd.PersistentFlags().StringVar(
			&cfg.KubeNamespace,
			"kube-namespace", "",
			"name of the kubernetes namespace to use")
	}
}

var (
	// clusterIdentifier provides a Prometheus metric that should uniquely identify the
	// cluster that any stolon component is associated with. Users can then join between
	// various metric series for the same cluster without making assumptions about service
	// discovery labels.
	clusterIdentifier = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stolon_cluster_identifier",
			Help: "Set to 1, is labelled with the cluster_name and component",
		},
		[]string{"cluster_name", "component"},
	)
)

func init() {
	prometheus.MustRegister(clusterIdentifier)
}

// CheckCommonConfig is a function that checks if two configs match and if not returns an error
func CheckCommonConfig(cfg *CommonConfig) error {
	if cfg.ClusterName == "" {
		return errors.New("cluster name required")
	}
	if cfg.StoreBackend == "" {
		return errors.New("store backend type required")
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
			return errors.New("unspecified kubernetes resource kind")
		}
		if cfg.KubeResourceKind != "configmap" {
			return fmt.Errorf("wrong kubernetes resource kind: %q", cfg.KubeResourceKind)
		}
	default:
		return fmt.Errorf("Unknown store backend: %q", cfg.StoreBackend)
	}

	return nil
}

// SetMetrics should be called by any stolon component that outputs application metrics.
// It sets the clusterIdentifier metric, which is key to joining across all the other
// metric series.
func SetMetrics(cfg *CommonConfig, component string) {
	clusterIdentifier.WithLabelValues(cfg.ClusterName, component).Set(1)
}

// IsColorLoggerEnable is a function which checks if logs should be colorized or not
func IsColorLoggerEnable(cmd *cobra.Command, cfg *CommonConfig) bool {
	if cmd.PersistentFlags().Changed("log-color") {
		return cfg.LogColor
	}
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}

// NewKVStore returns a new KVStore function object
func NewKVStore(ctx context.Context, cfg *CommonConfig) (store.KVStore, error) {
	return store.NewKVStore(
		ctx,
		store.Config{
			Backend:       store.BackendType(cfg.StoreBackend),
			Endpoints:     cfg.StoreEndpoints,
			Timeout:       cfg.StoreTimeout,
			CertFile:      cfg.StoreCertFile,
			KeyFile:       cfg.StoreKeyFile,
			CAFile:        cfg.StoreCAFile,
			SkipTLSVerify: cfg.StoreSkipTLSVerify,
		})
}

// NewStore is function that returns a new store object
func NewStore(ctx context.Context, cfg *CommonConfig) (store.Store, error) {
	var s store.Store

	switch cfg.StoreBackend {
	case "consul":
		fallthrough
	case "etcdv2":
		fallthrough
	case "etcdv3":
		storePath := filepath.Join(cfg.StorePrefix, cfg.ClusterName)

		kvstore, err := NewKVStore(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("NewStore: cannot create etcdv3 store: %v", err)
		}
		s = store.NewKVBackedStore(kvstore, storePath)
	case "kubernetes":
		kubecli, podName, namespace, err := getKubeValues(cfg)
		if err != nil {
			return nil, err
		}
		s, err = store.NewKubeStore(kubecli, podName, namespace, cfg.ClusterName)
		if err != nil {
			return nil, fmt.Errorf("NewStore: cannot create k8s store: %v", err)
		}
	}

	return s, nil
}

// NewElection is  function that returns a new election object
func NewElection(ctx context.Context, cfg *CommonConfig, uid string) (store.Election, error) {
	var election store.Election

	switch cfg.StoreBackend {
	case "consul":
		fallthrough
	case "etcdv2":
		fallthrough
	case "etcdv3":
		storePath := filepath.Join(cfg.StorePrefix, cfg.ClusterName)

		kvstore, err := NewKVStore(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("NewElection: cannot create kv store: %v", err)
		}
		election = store.NewKVBackedElection(
			kvstore,
			filepath.Join(storePath,
				common.SentinelLeaderKey),
			uid,
			cfg.StoreTimeout)
	case "kubernetes":
		kubecli, podName, namespace, err := getKubeValues(cfg)
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

func getKubeValues(cfg *CommonConfig) (*kubernetes.Clientset, string, string, error) {
	kubeClientConfig := util.NewKubeClientConfig(cfg.KubeConfig, cfg.KubeContext, cfg.KubeNamespace)
	kubecfg, err := kubeClientConfig.ClientConfig()
	if err != nil {
		return nil, "", "", err
	}
	kubecfg.Timeout = cfg.StoreTimeout
	kubecli, err := kubernetes.NewForConfig(kubecfg)
	if err != nil {
		return nil, "", "", fmt.Errorf("cannot create kubernetes client: %v", err)
	}
	var podName string
	if !cfg.IsStolonCtl {
		podName, err = util.PodName()
		if err != nil {
			return nil, "", "", err
		}
	}
	namespace, _, err := kubeClientConfig.Namespace()
	if err != nil {
		return nil, "", "", err
	}
	return kubecli, podName, namespace, nil
}
