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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/gravitational/stolon/common"
	"github.com/gravitational/stolon/pkg/cluster"
	"github.com/gravitational/stolon/pkg/store"

	kvstore "github.com/docker/libkv/store"
	"github.com/gravitational/trace"
	"k8s.io/kubernetes/pkg/util/strategicpatch"
)

func newClient(cfg config) (*client, error) {
	kvstore, err := store.NewStore(
		store.Backend(cfg.storeBackend),
		cfg.storeEndpoints,
		cfg.storeCertFile,
		cfg.storeKeyFile,
		cfg.storeCACertFile,
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &client{cfg: cfg, store: kvstore}, nil
}

type config struct {
	storeBackend    string
	storeEndpoints  string
	storeCertFile   string
	storeKeyFile    string
	storeCACertFile string
}

type clusterStatus struct {
	Sentinels      cluster.SentinelsInfo
	LeadSentinelID string
	Proxies        cluster.ProxiesInfo
}

type client struct {
	cfg   config
	store kvstore.Store
}

type clusterClient struct {
	*store.StoreManager
	client      *client
	clusterName string
}

func (c *client) getCluster(clusterName string) (*clusterClient, error) {
	if clusterName == "" {
		return nil, trace.BadParameter("please supply cluster name")
	}
	return &clusterClient{
		client:      c,
		clusterName: clusterName,
		StoreManager: store.NewStoreManager(c.store,
			filepath.Join(common.StoreBasePath, clusterName)),
	}, nil
}

func (c *client) Clusters() ([]string, error) {
	clusters := []string{}
	pairs, err := c.store.List(common.StoreBasePath)
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, trace.Wrap(err)
		}
		return clusters, nil
	}
	for _, pair := range pairs {
		clusters = append(clusters, filepath.Base(pair.Key))
	}
	sort.Strings(clusters)
	return clusters, nil
}

func (c *clusterClient) Config() (*cluster.NilConfig, error) {
	cv, _, err := c.GetClusterView()
	if err != nil {
		return nil, trace.Wrap(err, "cannot get clusterview for %v", c.clusterName)
	}
	if cv == nil {
		return nil, trace.NotFound("no clusterview available for %v", c.clusterName)
	}
	cfg := cv.Config
	if cfg == nil {
		return nil, trace.NotFound("no cluster config found for %v", c.clusterName)
	}
	return cfg, nil
}

func (c *clusterClient) PatchConfig(newData []byte) error {
	currentConfig, err := c.Config()
	if err != nil {
		return trace.Wrap(err, "can not get config for %v", c.clusterName)
	}
	currentData, err := json.Marshal(currentConfig)
	if err != nil {
		return trace.Wrap(err, "failed to marshal config")
	}
	patched, err := strategicpatch.StrategicMergePatch(currentData, newData, &cluster.NilConfig{})
	if err != nil {
		return trace.Wrap(err, "failed to merge patch config")
	}
	err = c.ReplaceConfig(patched)
	return trace.Wrap(err)
}

func (c *clusterClient) ReplaceConfig(data []byte) error {
	sid, err := c.GetLeaderSentinelId()
	if err != nil {
		return trace.Wrap(err)
	}
	sentinel, _, err := c.GetSentinelInfo(sid)
	if err != nil {
		return trace.Wrap(err)
	}
	if sentinel == nil {
		return trace.NotFound("leader sentinel info not available")
	}
	req, err := http.NewRequest("PUT",
		fmt.Sprintf("http://%s:%s/config/current",
			sentinel.ListenAddress,
			sentinel.Port), bytes.NewReader(data))
	if err != nil {
		return trace.Wrap(err, "cannot create request")
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return trace.Wrap(err, "error setting config")
	}
	if res.StatusCode != http.StatusOK {
		return trace.BadParameter("error setting config: leader sentinel returned non ok code: %s",
			res.Status)
	}
	return nil
}
