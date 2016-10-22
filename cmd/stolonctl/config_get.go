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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/store"

	"github.com/spf13/cobra"
)

var cmdConfigGet = &cobra.Command{
	Use:   "get",
	Run:   configGet,
	Short: "get configuration",
}

func init() {
	cmdConfig.AddCommand(cmdConfigGet)
}

func getConfig(e *store.StoreManager) (*cluster.NilConfig, error) {
	cd, _, err := e.GetClusterData()
	if err != nil {
		return nil, fmt.Errorf("cannot get clusterdata: %v", err)
	}
	if cd == nil {
		return nil, fmt.Errorf("nil cluster data: %v", err)
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		return nil, fmt.Errorf("unsupported clusterdata format version %d", cd.FormatVersion)
	}
	cv := cd.ClusterView
	if cv == nil {
		return nil, fmt.Errorf("no clusterview available")
	}
	cfg := cv.Config
	if cfg == nil {
		return nil, nil
	}
	return cfg, nil
}

func configGet(cmd *cobra.Command, args []string) {
	storePath := filepath.Join(common.StoreBasePath, cfg.clusterName)

	kvstore, err := store.NewStore(
		store.Backend(cfg.storeBackend),
		cfg.storeEndpoints,
		cfg.storeCertFile,
		cfg.storeKeyFile,
		cfg.storeCACertFile,
	)
	if err != nil {
		die("cannot create store: %v", err)
	}
	e := store.NewStoreManager(kvstore, storePath)

	cfg, err := getConfig(e)
	if err != nil {
		die("error: %v", err)
	}

	if cfg == nil {
		stdout("config is not defined")
		os.Exit(0)
	}

	cfgj, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		die("failed to marshall config: %v", err)
	}

	stdout(string(cfgj))
}
