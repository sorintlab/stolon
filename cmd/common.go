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

	"github.com/sorintlab/stolon/common"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type CommonConfig struct {
	StoreBackend           string
	StoreEndpoints         string
	StorePrefix            string
	StoreCertFile          string
	StoreKeyFile           string
	StoreCAFile            string
	StoreSkipTlsVerify     bool
	ClusterName            string
	InitialClusterSpecFile string
	LogColor               bool
	LogLevel               string
	Debug                  bool
}

func AddCommonFlags(cmd *cobra.Command, cfg *CommonConfig, hasLogger bool) {
	cmd.PersistentFlags().StringVar(&cfg.StoreBackend, "store-backend", "", "store backend type (etcdv2/etcd, etcdv3 or consul)")
	cmd.PersistentFlags().StringVar(&cfg.StoreEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (use https scheme for tls communication) (defaults: http://127.0.0.1:2379 for etcd, http://127.0.0.1:8500 for consul)")
	cmd.PersistentFlags().StringVar(&cfg.StorePrefix, "store-prefix", common.StorePrefix, "the store base prefix")
	cmd.PersistentFlags().StringVar(&cfg.StoreCertFile, "store-cert-file", "", "certificate file for client identification to the store")
	cmd.PersistentFlags().StringVar(&cfg.StoreKeyFile, "store-key", "", "private key file for client identification to the store")
	cmd.PersistentFlags().BoolVar(&cfg.StoreSkipTlsVerify, "store-skip-tls-verify", false, "skip store certificate verification (insecure!!!)")
	cmd.PersistentFlags().StringVar(&cfg.StoreCAFile, "store-ca-file", "", "verify certificates of HTTPS-enabled store servers using this CA bundle")
	cmd.PersistentFlags().StringVar(&cfg.ClusterName, "cluster-name", "", "cluster name")
	if hasLogger {
		cmd.PersistentFlags().BoolVar(&cfg.LogColor, "log-color", false, "enable color in log output (default if attached to a terminal)")
		cmd.PersistentFlags().StringVar(&cfg.LogLevel, "log-level", "info", "debug, info (default), warn or error")
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
