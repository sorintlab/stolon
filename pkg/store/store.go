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

package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"

	"github.com/coreos/pkg/capnslog"
	"github.com/docker/libkv"
	kvstore "github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	"github.com/docker/libkv/store/etcd"
)

func init() {
	etcd.Register()
	consul.Register()
}

var log = capnslog.NewPackageLogger("github.com/sorintlab/stolon/pkg", "store")

// Backend represents a KV Store Backend
type Backend string

const (
	CONSUL Backend = "consul"
	ETCD   Backend = "etcd"
)

const (
	keepersDiscoveryInfoDir = "/keepers/discovery/"
	clusterDataFile         = "clusterdata"
	leaderSentinelInfoFile  = "/sentinels/leaderinfo"
	sentinelsInfoDir        = "/sentinels/info/"
	proxiesInfoDir          = "/proxies/info/"
)

const (
	DefaultEtcdEndpoints   = "127.0.0.1:2379"
	DefaultConsulEndpoints = "127.0.0.1:8500"
)

const (
	//TODO(sgotti) fix this in libkv?
	// consul min ttl is 10s and libkv divides this by 2
	MinTTL = 20 * time.Second
)

type StoreManager struct {
	clusterPath string
	store       kvstore.Store
}

func NewStore(backend Backend, addrsStr string) (kvstore.Store, error) {

	var kvbackend kvstore.Backend
	switch backend {
	case CONSUL:
		kvbackend = kvstore.CONSUL
	case ETCD:
		kvbackend = kvstore.ETCD
	default:
		return nil, fmt.Errorf("Unknown store backend: %q", backend)
	}

	if addrsStr == "" {
		switch backend {
		case CONSUL:
			addrsStr = DefaultConsulEndpoints
		case ETCD:
			addrsStr = DefaultEtcdEndpoints
		}
	}
	addrs := strings.Split(addrsStr, ",")

	store, err := libkv.NewStore(kvbackend, addrs, &kvstore.Config{ConnectionTimeout: 10 * time.Second})
	if err != nil {
		return nil, err
	}
	return store, nil
}

func NewStoreManager(kvStore kvstore.Store, path string) *StoreManager {
	return &StoreManager{
		clusterPath: path,
		store:       kvStore,
	}
}

func (e *StoreManager) SetClusterData(mss cluster.KeepersState, cv *cluster.ClusterView, previous *kvstore.KVPair) (*kvstore.KVPair, error) {
	// write cluster view
	cd := &cluster.ClusterData{
		KeepersState: mss,
		ClusterView:  cv,
	}
	cdj, err := json.Marshal(cd)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(e.clusterPath, clusterDataFile)
	_, pair, err := e.store.AtomicPut(path, cdj, previous, nil)
	return pair, err
}

func (e *StoreManager) GetClusterData() (*cluster.ClusterData, *kvstore.KVPair, error) {
	var cd *cluster.ClusterData
	path := filepath.Join(e.clusterPath, clusterDataFile)
	pair, err := e.store.Get(path)
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, nil, err
		}
		return nil, nil, nil
	}
	if err := json.Unmarshal(pair.Value, &cd); err != nil {
		return nil, nil, err
	}
	return cd, pair, nil
}

func (e *StoreManager) SetKeeperDiscoveryInfo(id string, ms *cluster.KeeperDiscoveryInfo, ttl time.Duration) error {
	msj, err := json.Marshal(ms)
	if err != nil {
		return err
	}
	if ttl < MinTTL {
		ttl = MinTTL
	}
	return e.store.Put(filepath.Join(e.clusterPath, keepersDiscoveryInfoDir, id), msj, &kvstore.WriteOptions{TTL: ttl})
}

func (e *StoreManager) GetKeeperDiscoveryInfo(id string) (*cluster.KeeperDiscoveryInfo, bool, error) {
	if id == "" {
		return nil, false, fmt.Errorf("empty keeper id")
	}
	var keeper cluster.KeeperDiscoveryInfo
	pair, err := e.store.Get(filepath.Join(e.clusterPath, keepersDiscoveryInfoDir, id))
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, false, err
		}
		return nil, false, nil
	}
	if err := json.Unmarshal(pair.Value, &keeper); err != nil {
		return nil, false, err
	}
	return &keeper, true, nil
}

func (e *StoreManager) GetKeepersDiscoveryInfo() (cluster.KeepersDiscoveryInfo, error) {
	keepers := cluster.KeepersDiscoveryInfo{}
	pairs, err := e.store.List(filepath.Join(e.clusterPath, keepersDiscoveryInfoDir))
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, err
		}
		return keepers, nil
	}
	for _, pair := range pairs {
		var keeper cluster.KeeperDiscoveryInfo
		err = json.Unmarshal(pair.Value, &keeper)
		if err != nil {
			return nil, err
		}
		keepers = append(keepers, &keeper)
	}
	return keepers, nil
}

func (e *StoreManager) SetSentinelInfo(si *cluster.SentinelInfo, ttl time.Duration) error {
	sij, err := json.Marshal(si)
	if err != nil {
		return err
	}
	if ttl < MinTTL {
		ttl = MinTTL
	}
	return e.store.Put(filepath.Join(e.clusterPath, sentinelsInfoDir, si.ID), sij, &kvstore.WriteOptions{TTL: ttl})
}

func (e *StoreManager) GetSentinelInfo(id string) (*cluster.SentinelInfo, bool, error) {
	if id == "" {
		return nil, false, fmt.Errorf("empty sentinel id")
	}
	var si cluster.SentinelInfo
	pair, err := e.store.Get(filepath.Join(e.clusterPath, sentinelsInfoDir, id))
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, false, err
		}
		return nil, false, nil
	}
	err = json.Unmarshal(pair.Value, &si)
	if err != nil {
		return nil, false, err
	}
	return &si, true, nil
}

func (e *StoreManager) GetSentinelsInfo() (cluster.SentinelsInfo, error) {
	ssi := cluster.SentinelsInfo{}
	pairs, err := e.store.List(filepath.Join(e.clusterPath, sentinelsInfoDir))
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, err
		}
		return ssi, nil
	}
	for _, pair := range pairs {
		var si cluster.SentinelInfo
		err = json.Unmarshal(pair.Value, &si)
		if err != nil {
			return nil, err
		}
		ssi = append(ssi, &si)
	}
	return ssi, nil
}

func (e *StoreManager) GetLeaderSentinelId() (string, error) {
	pair, err := e.store.Get(filepath.Join(e.clusterPath, common.SentinelLeaderKey))
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return "", err
		}
		return "", nil
	}
	return string(pair.Value), nil
}

func (e *StoreManager) SetProxyInfo(pi *cluster.ProxyInfo, ttl time.Duration) error {
	pij, err := json.Marshal(pi)
	if err != nil {
		return err
	}
	if ttl < MinTTL {
		ttl = MinTTL
	}
	return e.store.Put(filepath.Join(e.clusterPath, proxiesInfoDir, pi.ID), pij, &kvstore.WriteOptions{TTL: ttl})
}

func (e *StoreManager) GetProxyInfo(id string) (*cluster.ProxyInfo, bool, error) {
	if id == "" {
		return nil, false, fmt.Errorf("empty proxy id")
	}
	var pi cluster.ProxyInfo
	pair, err := e.store.Get(filepath.Join(e.clusterPath, proxiesInfoDir, id))
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, false, err
		}
		return nil, false, nil
	}
	err = json.Unmarshal(pair.Value, &pi)
	if err != nil {
		return nil, false, err
	}
	return &pi, true, nil
}

func (e *StoreManager) GetProxiesInfo() (cluster.ProxiesInfo, error) {
	psi := cluster.ProxiesInfo{}
	pairs, err := e.store.List(filepath.Join(e.clusterPath, proxiesInfoDir))
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, err
		}
		return psi, nil
	}
	for _, pair := range pairs {
		var pi cluster.ProxyInfo
		err = json.Unmarshal(pair.Value, &pi)
		if err != nil {
			return nil, err
		}
		psi = append(psi, &pi)
	}
	return psi, nil
}
