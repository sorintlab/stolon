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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/flagutil"
	"github.com/sorintlab/stolon/pkg/store"

	kvstore "github.com/docker/libkv/store"
	"github.com/spf13/cobra"
)

const (
	maxRetries = 3
)

var cmdStolonCtl = &cobra.Command{
	Use:     "stolonctl",
	Short:   "stolon command line client",
	Version: cmd.Version,
	PersistentPreRun: func(c *cobra.Command, args []string) {
		if c.Name() != "stolonctl" && c.Name() != "version" {
			if err := cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
				die(err.Error())
			}
		}
	},
	// just defined to make --version work
	Run: func(c *cobra.Command, args []string) { c.Help() },
}

type config struct {
	cmd.CommonConfig
}

var cfg config

func init() {
	cmd.AddCommonFlags(cmdStolonCtl, &cfg.CommonConfig)
}

var cmdVersion = &cobra.Command{
	Use:   "version",
	Run:   versionCommand,
	Short: "Display the version",
}

func init() {
	cmdStolonCtl.AddCommand(cmdVersion)
}

func versionCommand(c *cobra.Command, args []string) {
	stdout("stolonctl version %s", cmd.Version)
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

func NewStore() (*store.StoreManager, error) {
	storePath := filepath.Join(common.StoreBasePath, cfg.ClusterName)

	kvstore, err := store.NewStore(store.Config{
		Backend:       store.Backend(cfg.StoreBackend),
		Endpoints:     cfg.StoreEndpoints,
		CertFile:      cfg.StoreCertFile,
		KeyFile:       cfg.StoreKeyFile,
		CAFile:        cfg.StoreCAFile,
		SkipTLSVerify: cfg.StoreSkipTlsVerify,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}
	return store.NewStoreManager(kvstore, storePath), nil
}

func getClusterData(e *store.StoreManager) (*cluster.ClusterData, *kvstore.KVPair, error) {
	cd, pair, err := e.GetClusterData()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get cluster data: %v", err)
	}
	if cd == nil {
		return nil, nil, fmt.Errorf("nil cluster data: %v", err)
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		return nil, nil, fmt.Errorf("unsupported cluster data format version %d", cd.FormatVersion)
	}
	if err := cd.Cluster.Spec.Validate(); err != nil {
		return nil, nil, fmt.Errorf("clusterdata validation failed: %v", err)
	}
	return cd, pair, nil
}

func askConfirmation(message string) (bool, error) {
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, message)
		input, err := in.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("error reading input: %v", err)
		}
		switch input {
		case "yes\n":
			return true, nil
		case "no\n":
			return false, nil
		default:
			stdout("Please enter 'yes' or 'no'")
		}
	}
}
