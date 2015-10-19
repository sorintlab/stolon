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
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sorintlab/stolon/common"
	etcdm "github.com/sorintlab/stolon/pkg/etcd"

	etcd "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/etcd/client"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/spf13/cobra"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
)

var cmdListClusters = &cobra.Command{
	Use: "list-clusters",
	Run: listClusters,
}

func init() {
	cmdStolonCtl.AddCommand(cmdListClusters)
}

func getClusters(etcdBasePath string) ([]string, error) {
	eCfg := etcd.Config{
		Transport: &http.Transport{},
		Endpoints: strings.Split(cfg.etcdEndpoints, ","),
	}
	eClient, err := etcd.New(eCfg)
	if err != nil {
		return nil, err
	}
	kAPI := etcd.NewKeysAPI(eClient)

	clusters := []string{}
	res, err := kAPI.Get(context.Background(), etcdBasePath, &etcd.GetOptions{Recursive: true, Quorum: true})
	if err != nil && !etcdm.IsEtcdNotFound(err) {
		return nil, err
	} else if !etcdm.IsEtcdNotFound(err) {
		for _, node := range res.Node.Nodes {
			clusters = append(clusters, filepath.Base(node.Key))
		}
	}
	sort.Strings(clusters)
	return clusters, nil
}

func listClusters(cmd *cobra.Command, args []string) {
	clusters, err := getClusters(common.EtcdBasePath)
	if err != nil {
		die("cannot get clusters: %v", err)
	}

	for _, cluster := range clusters {
		stdout(cluster)
	}
}
