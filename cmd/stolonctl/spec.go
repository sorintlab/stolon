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
	"path/filepath"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/store"

	"github.com/spf13/cobra"
)

var cmdSpec = &cobra.Command{
	Use:   "spec",
	Run:   spec,
	Short: "Retrieve the current cluster specification",
}

func init() {
	cmdStolonCtl.AddCommand(cmdSpec)
}

func spec(cmd *cobra.Command, args []string) {
	storePath := filepath.Join(common.StoreBasePath, cfg.clusterName)

	kvstore, err := store.NewStore(store.Backend(cfg.storeBackend), cfg.storeEndpoints)
	if err != nil {
		die("cannot create store: %v", err)
	}
	e := store.NewStoreManager(kvstore, storePath)

	cd, _, err := getClusterData(e)
	if err != nil {
		die("%v", err)
	}
	if cd.Cluster == nil {
		die("no cluster spec available")
	}
	if cd.Cluster.Spec == nil {
		die("no cluster spec available")
	}
	specj, err := json.MarshalIndent(cd.Cluster.Spec, "", "\t")
	if err != nil {
		die("failed to marshall spec: %v", err)
	}

	stdout("%s", specj)
}
