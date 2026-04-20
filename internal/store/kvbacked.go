// Copyright 2026 PgVillage
// Copyright 2018 Sorint.lab
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
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kvtools/consul"
	"github.com/kvtools/etcdv2"
	"github.com/kvtools/etcdv3"
	"github.com/kvtools/valkeyrie"
	cluster "github.com/pgvillage-tools/stolon/api/v1"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/logging"
)

// BackendType represents a type of KV Store BackendType
type BackendType string

const (
	// CONSUL means that consul is used as backend
	CONSUL BackendType = consul.StoreName
	// ETCDV2 means that etcd is used as backend and that the v2 api is used
	ETCDV2 BackendType = etcdv2.StoreName
	// ETCDV3 means that etcd is used as backend and that the v3 api is used
	ETCDV3 BackendType = etcdv3.StoreName
)

var storeTypes = []string{
	consul.StoreName,
	etcdv2.StoreName,
	etcdv3.StoreName,
}

func (bt BackendType) string() string {
	return string(bt)
}

const (
	keepersInfoDir   = "/keepers/info/"
	clusterDataFile  = "clusterdata"
	sentinelsInfoDir = "/sentinels/info/"
	proxiesInfoDir   = "/proxies/info/"
)

const (
	// DefaultEtcdEndpoints defines the default endpoints when using etcd
	DefaultEtcdEndpoints = "http://127.0.0.1:2379"
	// DefaultConsulEndpoints defines the default endpoints when using consul
	DefaultConsulEndpoints = "http://127.0.0.1:8500"
)

// TODO(sgotti) fix this in libkv?
// consul min ttl is 10s and libkv divides this by 2
const minTTL = 20 * time.Second
const dialTimeout = 20 * time.Second

var urlSchemeRegexp = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+-.]*)://`)

// Config defines the config to be used for the endpoint
type Config struct {
	Backend       BackendType
	Endpoints     string
	Timeout       time.Duration
	BasePath      string
	CertFile      string
	KeyFile       string
	CAFile        string
	SkipTLSVerify bool
}

// TLSConfig creates and returns a TLSConfig from a Config if applicable
func (c Config) TLSConfig() (*tls.Config, error) {
	scheme, err := c.EndpointScheme()
	if err != nil {
		return nil, err
	}
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("endpoints scheme must be http or https")
	}
	if scheme != "https" {
		return nil, nil
	}

	tlsConfig, err := common.NewTLSConfig(c.CertFile, c.KeyFile, c.CAFile, c.SkipTLSVerify)
	if err != nil {
		return nil, fmt.Errorf("cannot create store tls config: %v", err)
	}
	return tlsConfig, nil
}

// EndpointScheme returns the scheme of all endpoints
// (they should all have the same scheme, or an error occurs)
func (c Config) EndpointScheme() (string, error) {
	endpoints, err := c.EndPoints()
	if err != nil {
		return "", err
	}
	var scheme string
	for _, e := range endpoints {
		var curscheme string
		if urlSchemeRegexp.Match([]byte(e)) {
			u, err := url.Parse(e)
			if err != nil {
				return "", fmt.Errorf("cannot parse endpoint %q: %v", e, err)
			}
			curscheme = u.Scheme
		} else {
			// Assume it's a schemeless endpoint
			curscheme = "http"
		}
		if scheme == "" {
			scheme = curscheme
		}
		if scheme != curscheme {
			return "", errors.New("all the endpoints must have the same scheme")
		}
	}
	return scheme, nil
}

// EndpointAddrs returns the addresses of the endpoints
func (c Config) EndpointAddrs() ([]string, error) {
	endpoints, err := c.EndPoints()
	if err != nil {
		return nil, err
	}
	addrs := []string{}
	var scheme string
	for _, e := range endpoints {
		var curscheme, addr string
		if urlSchemeRegexp.Match([]byte(e)) {
			u, err := url.Parse(e)
			if err != nil {
				return nil, fmt.Errorf("cannot parse endpoint %q: %v", e, err)
			}
			curscheme = u.Scheme
			addr = u.Host
		} else {
			// Assume it's a schemeless endpoint
			curscheme = "http"
			addr = e
		}
		if scheme == "" {
			scheme = curscheme
		}
		if scheme != curscheme {
			return nil, errors.New("all the endpoints must have the same scheme")
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// EndPoints returns a list of endpoints as strings
func (c Config) EndPoints() ([]string, error) {
	endpointsStr := c.Endpoints
	if endpointsStr == "" {
		switch c.Backend {
		case CONSUL:
			endpointsStr = DefaultConsulEndpoints
		case ETCDV2, ETCDV3:
			endpointsStr = DefaultEtcdEndpoints
		default:
			return nil, fmt.Errorf(
				"unexpected store '%s', should be any of %v",
				c.Backend,
				strings.Join(storeTypes, ","),
			)
		}
	}
	return strings.Split(endpointsStr, ","), nil
}

// KVPair represents {Key, Value, Lastindex} tuple
type KVPair struct {
	Key       string
	Value     []byte
	LastIndex uint64
}

// WriteOptions defines options to be used when writing to the backend
type WriteOptions struct {
	TTL time.Duration
}

// KVStore is an interface representing a backend (e.a. etcd, k8s en Consul)
type KVStore interface {
	// Put a value at the specified key
	Put(ctx context.Context, key string, value []byte, options *WriteOptions) error

	// Get a value given its key
	Get(ctx context.Context, key string) (*KVPair, error)

	// List the content of a given prefix
	List(ctx context.Context, directory string) ([]*KVPair, error)

	// Atomic CAS operation on a single value.
	// Pass previous = nil to create a new key.
	AtomicPut(ctx context.Context, key string, value []byte, previous *KVPair, options *WriteOptions) (*KVPair, error)

	Delete(ctx context.Context, key string) error

	// Close the store connection
	Close() error
}

// NewKVStore returns a freshly initialized KVStore
func NewKVStore(ctx context.Context, cfg Config) (KVStore, error) {
	ctx, logger := logging.GetLogComponent(ctx, logging.StoreComponent)
	addrs, err := cfg.EndpointAddrs()
	if err != nil {
		return nil, err
	}
	logger.Debug().Any("endpoints", addrs).Msg("")
	tlsConfig, err := cfg.TLSConfig()
	if err != nil {
		return nil, err
	}
	logger.Debug().Bool("tls enabled", tlsConfig != nil).Msg("")

	var config valkeyrie.Config
	switch cfg.Backend {
	case CONSUL:
		config = &consul.Config{
			TLS:               tlsConfig,
			ConnectionTimeout: cfg.Timeout,
		}
	case ETCDV2:
		config = &etcdv2.Config{
			TLS:               tlsConfig,
			ConnectionTimeout: cfg.Timeout,
		}

	case ETCDV3:
		config = &etcdv3.Config{
			TLS:               tlsConfig,
			ConnectionTimeout: cfg.Timeout,
			SyncPeriod:        cfg.Timeout,
		}
	default:
		return nil, fmt.Errorf("Unknown store backend: %q", cfg.Backend)
	}
	logger.Debug().Any("store config", config).Msg("")

	store, err := valkeyrie.NewStore(ctx, cfg.Backend.string(), addrs, config)
	if err != nil {
		return nil, err
	}
	return &libKVStore{store: store}, nil
}

// KVBackedStore defines a config store backed by a KV backend
type KVBackedStore struct {
	clusterPath string
	store       KVStore
}

// NewKVBackedStore returns a freshly initialized KVBackedStore
func NewKVBackedStore(kvStore KVStore, path string) *KVBackedStore {
	return &KVBackedStore{
		clusterPath: path,
		store:       kvStore,
	}
}

// AtomicPutClusterData is an atomic way to store CLusterData to a kv store
func (s *KVBackedStore) AtomicPutClusterData(ctx context.Context, cd *cluster.Data, previous *KVPair) (*KVPair, error) {
	cdj, err := json.Marshal(cd)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(s.clusterPath, clusterDataFile)
	// Skip prev Value since LastIndex is enough for a CAS and it gives
	// problem with etcd v2 api with big prev values.
	var prev *KVPair
	if previous != nil {
		prev = &KVPair{
			Key:       previous.Key,
			LastIndex: previous.LastIndex,
		}
	}
	return s.store.AtomicPut(ctx, path, cdj, prev, nil)
}

// PutClusterData stores ClusterData to a kv store
func (s *KVBackedStore) PutClusterData(ctx context.Context, cd *cluster.Data) error {
	cdj, err := json.Marshal(cd)
	if err != nil {
		return err
	}
	path := filepath.Join(s.clusterPath, clusterDataFile)
	return s.store.Put(ctx, path, cdj, nil)
}

// GetClusterData retrieves ClusterData from a kv store
func (s *KVBackedStore) GetClusterData(ctx context.Context) (*cluster.Data, *KVPair, error) {
	var cd *cluster.Data
	path := filepath.Join(s.clusterPath, clusterDataFile)
	pair, err := s.store.Get(ctx, path)
	if err != nil {
		if err != ErrKeyNotFound {
			return nil, nil, err
		}
		return nil, nil, nil
	}
	if err := json.Unmarshal(pair.Value, &cd); err != nil {
		return nil, nil, err
	}
	return cd, pair, nil
}

// SetKeeperInfo stores keeper info to a kv store
func (s *KVBackedStore) SetKeeperInfo(ctx context.Context, id string, ms *cluster.KeeperInfo, ttl time.Duration) error {
	msj, err := json.Marshal(ms)
	if err != nil {
		return err
	}
	if ttl < minTTL {
		ttl = minTTL
	}
	return s.store.Put(ctx, filepath.Join(s.clusterPath, keepersInfoDir, id), msj, &WriteOptions{TTL: ttl})
}

// GetKeepersInfo retrieves all keeper info from a kv store
func (s *KVBackedStore) GetKeepersInfo(ctx context.Context) (cluster.KeepersInfo, error) {
	keepers := cluster.KeepersInfo{}
	pairs, err := s.store.List(ctx, filepath.Join(s.clusterPath, keepersInfoDir))
	if err != nil {
		if err != ErrKeyNotFound {
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

// SetSentinelInfo stores info on a sentinel to the kv store
func (s *KVBackedStore) SetSentinelInfo(ctx context.Context, si *cluster.SentinelInfo, ttl time.Duration) error {
	sij, err := json.Marshal(si)
	if err != nil {
		return err
	}
	if ttl < minTTL {
		ttl = minTTL
	}
	return s.store.Put(ctx, filepath.Join(s.clusterPath, sentinelsInfoDir, si.UID), sij, &WriteOptions{TTL: ttl})
}

// GetSentinelsInfo retrieves all sentinel info from a kv store
func (s *KVBackedStore) GetSentinelsInfo(ctx context.Context) (cluster.SentinelsInfo, error) {
	ssi := cluster.SentinelsInfo{}
	pairs, err := s.store.List(ctx, filepath.Join(s.clusterPath, sentinelsInfoDir))
	if err != nil {
		if err != ErrKeyNotFound {
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

// SetProxyInfo stores info on a proxy to the kv store
func (s *KVBackedStore) SetProxyInfo(ctx context.Context, pi *cluster.ProxyInfo, ttl time.Duration) error {
	pij, err := json.Marshal(pi)
	if err != nil {
		return err
	}
	if ttl < minTTL {
		ttl = minTTL
	}
	return s.store.Put(ctx, filepath.Join(s.clusterPath, proxiesInfoDir, pi.UID), pij, &WriteOptions{TTL: ttl})
}

// GetProxiesInfo retrieves all proxy info from a kv store
func (s *KVBackedStore) GetProxiesInfo(ctx context.Context) (cluster.ProxiesInfo, error) {
	psi := cluster.ProxiesInfo{}
	pairs, err := s.store.List(ctx, filepath.Join(s.clusterPath, proxiesInfoDir))
	if err != nil {
		if err != ErrKeyNotFound {
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
		psi[pi.UID] = &pi
	}
	return psi, nil
}

// NewKVBackedElection starts a campaign for getting elected with a kv backed store
func NewKVBackedElection(kvStore KVStore, path, candidateUID string, timeout time.Duration) Election {
	switch kvStore := kvStore.(type) {
	case *libKVStore:
		s := kvStore
		candidate := NewCandidate(s.store, path, candidateUID, minTTL)
		return &libkvElection{store: s, path: path, candidate: candidate}
	case *etcdV3Store:
		etcdV3Store := kvStore
		return &etcdv3Election{
			c:              etcdV3Store.c,
			path:           path,
			candidateUID:   candidateUID,
			ttl:            minTTL,
			requestTimeout: timeout,
		}
	default:
		panic("unknown kvstore")
	}
}
