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
	"os"
	"strings"

	"github.com/sorintlab/stolon/pkg/flagutil"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/spf13/cobra"
)

var cmdStolonCtl = &cobra.Command{
	Use:   "stolonctl",
	Short: "stolon command line client",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if cfg.clusterName == "" {
			die("cluster name required")
		}
		if cfg.storeBackend == "" {
			die("store backend type required")
		}
	},
}

type config struct {
	storeBackend    string
	storeEndpoints  string
	storeCertFile   string
	storeKeyFile    string
	storeCACertFile string
	clusterName     string
}

var cfg config

func init() {
	cmdStolonCtl.PersistentFlags().StringVar(&cfg.storeBackend, "store-backend", "", "store backend type (etcd or consul)")
	cmdStolonCtl.PersistentFlags().StringVar(&cfg.storeEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (defaults: 127.0.0.1:2379 for etcd, 127.0.0.1:8500 for consul)")
	cmdStolonCtl.PersistentFlags().StringVar(&cfg.storeCertFile, "store-cert", "", "path to the client server TLS cert file")
	cmdStolonCtl.PersistentFlags().StringVar(&cfg.storeKeyFile, "store-key", "", "path to the client server TLS key file")
	cmdStolonCtl.PersistentFlags().StringVar(&cfg.storeCACertFile, "store-cacert", "", "path to the client server TLS trusted CA key file")
	cmdStolonCtl.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
}

func main() {
	flagutil.SetFlagsFromEnv(cmdStolonCtl.PersistentFlags(), "STOLONCTL")

	cmdStolonCtl.Execute()
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
