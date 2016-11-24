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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"

	"github.com/docker/libkv"
	kvstore "github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	"github.com/docker/libkv/store/etcd"
)

func init() {
	etcd.Register()
	consul.Register()
}

// Backend represents a KV Store Backend
type Backend string

const (
	CONSUL Backend = "consul"
	ETCD   Backend = "etcd"
)

const (
	sentinelLeaderKey = "sentinel-leader"

	keepersInfoDir         = "/keepers/info/"
	clusterDataFile        = "clusterdata"
	leaderSentinelInfoFile = "/sentinels/leaderinfo"
	sentinelsInfoDir       = "/sentinels/info/"
	proxiesInfoDir         = "/proxies/info/"
)

const (
	DefaultEtcdEndpoints   = "http://127.0.0.1:2379"
	DefaultConsulEndpoints = "http://127.0.0.1:8500"
)

const (
	//TODO(sgotti) fix this in libkv?
	// consul min ttl is 10s and libkv divides this by 2
	MinTTL = 20 * time.Second
)

type Config struct {
	Backend       Backend
	Endpoints     string
	CertFile      string
	KeyFile       string
	CAFile        string
	SkipTLSVerify bool
}

type StoreManager struct {
	clusterPath string
	store       kvstore.Store
}

func NewStore(cfg Config) (kvstore.Store, error) {
	var kvBackend kvstore.Backend
	switch cfg.Backend {
	case CONSUL:
		kvBackend = kvstore.CONSUL
	case ETCD:
		kvBackend = kvstore.ETCD
	default:
		return nil, fmt.Errorf("Unknown store backend: %q", cfg.Backend)
	}

	endpointsStr := cfg.Endpoints
	if endpointsStr == "" {
		switch cfg.Backend {
		case CONSUL:
			endpointsStr = DefaultConsulEndpoints
		case ETCD:
			endpointsStr = DefaultEtcdEndpoints
		}
	}
	endpoints := strings.Split(endpointsStr, ",")

	// 1) since libkv wants endpoints as a list of IP and not URLs but we
	// want to also support them then parse and strip them
	// 2) since libkv will enable TLS for all endpoints when config.TLS
	// isn't nil we have to check that all the endpoints have the same
	// scheme
	addrs := []string{}
	var scheme string
	for _, e := range endpoints {
		var addr string
		u, err := url.Parse(e)
		if err != nil {
			return nil, fmt.Errorf("cannot parse endpoint %q: %v", e, err)
		}
		if u.Scheme == "" {
			u.Scheme = "http"
			addr = e
		} else {
			addr = u.Host
		}
		if scheme == "" {
			scheme = u.Scheme
		}
		if scheme != u.Scheme {
			return nil, fmt.Errorf("all the endpoints must have the same scheme")
		}
		addrs = append(addrs, addr)
	}

	var tlsConfig *tls.Config
	if scheme == "https" {
		var err error
		tlsConfig, err = common.NewTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.SkipTLSVerify)
		if err != nil {
			return nil, fmt.Errorf("cannot create store tls config: %v", err)
		}
	}

	config := &kvstore.Config{
		TLS:               tlsConfig,
		ConnectionTimeout: 10 * time.Second,
	}

	store, err := libkv.NewStore(kvBackend, addrs, config)
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

func (e *StoreManager) AtomicPutClusterData(cd *cluster.ClusterData, previous *kvstore.KVPair) (*kvstore.KVPair, error) {
	cdj, err := json.Marshal(cd)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(e.clusterPath, clusterDataFile)
	// Skip prev Value since LastIndex is enough for a CAS and it gives
	// problem with etcd v2 api with big prev values.
	var prev *kvstore.KVPair
	if previous != nil {
		prev = &kvstore.KVPair{
			Key:       previous.Key,
			LastIndex: previous.LastIndex,
		}
	}
	_, pair, err := e.store.AtomicPut(path, cdj, prev, nil)
	return pair, err
}

func (e *StoreManager) PutClusterData(cd *cluster.ClusterData) error {
	cdj, err := json.Marshal(cd)
	if err != nil {
		return err
	}
	path := filepath.Join(e.clusterPath, clusterDataFile)
	return e.store.Put(path, cdj, nil)
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

func (e *StoreManager) SetKeeperInfo(id string, ms *cluster.KeeperInfo, ttl time.Duration) error {
	msj, err := json.Marshal(ms)
	if err != nil {
		return err
	}
	if ttl < MinTTL {
		ttl = MinTTL
	}
	return e.store.Put(filepath.Join(e.clusterPath, keepersInfoDir, id), msj, &kvstore.WriteOptions{TTL: ttl})
}

func (e *StoreManager) GetKeeperInfo(id string) (*cluster.KeeperInfo, bool, error) {
	if id == "" {
		return nil, false, fmt.Errorf("empty keeper id")
	}
	var keeper cluster.KeeperInfo
	pair, err := e.store.Get(filepath.Join(e.clusterPath, keepersInfoDir, id))
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

func (e *StoreManager) GetKeepersInfo() (cluster.KeepersInfo, error) {
	keepers := cluster.KeepersInfo{}
	pairs, err := e.store.List(filepath.Join(e.clusterPath, keepersInfoDir))
	if err != nil {
		if err != kvstore.ErrKeyNotFound {
			return nil, err
		}
		return keepers, nil
	}
	for _, pair := range pairs {
		var ki cluster.KeeperInfo
		err = json.Unmarshal(pair.Value, &ki)
		if err != nil {
			return nil, err
		}
		keepers[ki.UID] = &ki
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
	return e.store.Put(filepath.Join(e.clusterPath, sentinelsInfoDir, si.UID), sij, &kvstore.WriteOptions{TTL: ttl})
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
	pair, err := e.store.Get(filepath.Join(e.clusterPath, sentinelLeaderKey))
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
	return e.store.Put(filepath.Join(e.clusterPath, proxiesInfoDir, pi.UID), pij, &kvstore.WriteOptions{TTL: ttl})
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
