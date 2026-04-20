// Copyright 2026 PgVillage
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
	"context"
	"errors"
	"time"

	cluster "github.com/pgvillage-tools/stolon/api/v1"
)

//go:generate mockgen -destination=../mock/store/store.go -source=$GOFILE

var (
	// ErrKeyNotFound is thrown when the key is not found in the store during a Get operation
	ErrKeyNotFound = errors.New("Key not found in store")
	// ErrKeyModified is thrown when the key was modified while perfroming the atomic operation
	ErrKeyModified = errors.New("Unable to complete atomic operation, key modified")
	// ErrElectionNoLeader is thrown when there is no leader
	ErrElectionNoLeader = errors.New("election: no leader")
)

// Store is an interface for a kv store which could be consul, or etcd (v2 or v3)
type Store interface {
	AtomicPutClusterData(ctx context.Context, cd *cluster.Data, previous *KVPair) (*KVPair, error)
	PutClusterData(ctx context.Context, cd *cluster.Data) error
	GetClusterData(ctx context.Context) (*cluster.Data, *KVPair, error)
	SetKeeperInfo(ctx context.Context, id string, ms *cluster.KeeperInfo, ttl time.Duration) error
	GetKeepersInfo(ctx context.Context) (cluster.KeepersInfo, error)
	SetSentinelInfo(ctx context.Context, si *cluster.SentinelInfo, ttl time.Duration) error
	GetSentinelsInfo(ctx context.Context) (cluster.SentinelsInfo, error)
	SetProxyInfo(ctx context.Context, pi *cluster.ProxyInfo, ttl time.Duration) error
	GetProxiesInfo(ctx context.Context) (cluster.ProxiesInfo, error)
}

// Election takes care of the election proces with a kv backend
type Election interface {
	// TODO(sgotti) this mimics the current docker/leadership API and the etcdv3
	// implementations adapt to it. In future it could be replaced with a better
	// api like the current one implemented by etcdclientv3/concurrency.
	//
	// WARNING: If the election error channel receives any error, it is vital that
	// the consuming code calls election.Stop(). Failure to do so can cause
	// subsequent elections to hang indefinitely across all participants of an
	// election.
	RunForElection(context.Context) (<-chan bool, <-chan error)
	Leader() (string, error)
	Stop()
}
