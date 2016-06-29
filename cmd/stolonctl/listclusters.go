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
	"path/filepath"
	"sort"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/store"

	libkvstore "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/docker/libkv/store"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/spf13/cobra"
)

var cmdListClusters = &cobra.Command{
	Use: "list-clusters",
	Run: listClusters,
}

func init() {
	cmdStolonCtl.AddCommand(cmdListClusters)
}

func getClusters(storeBasePath string) ([]string, error) {
	kvstore, err := store.NewStore(
		store.Backend(cfg.storeBackend),
		cfg.storeEndpoints,
		cfg.storeCertFile,
		cfg.storeKeyFile,
		cfg.storeCACertFile,
	)

	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	clusters := []string{}
	pairs, err := kvstore.List(storeBasePath)
	if err != nil {
		if err != libkvstore.ErrKeyNotFound {
			return nil, err
		}
		return clusters, nil
	}
	for _, pair := range pairs {
		clusters = append(clusters, filepath.Base(pair.Key))
	}
	sort.Strings(clusters)
	return clusters, nil
}

func listClusters(cmd *cobra.Command, args []string) {
	clusters, err := getClusters(common.StoreBasePath)
	if err != nil {
		die("cannot get clusters: %v", err)
	}

	for _, cluster := range clusters {
		stdout(cluster)
	}
}
