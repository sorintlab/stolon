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

	"github.com/sorintlab/stolon/pkg/cluster"
)

var (
	// ErrKeyNotFound is thrown when the key is not found in the store during a Get operation
	ErrKeyNotFound      = errors.New("Key not found in store")
	ErrKeyModified      = errors.New("Unable to complete atomic operation, key modified")
	ErrElectionNoLeader = errors.New("election: no leader")
)

type Store interface {
	AtomicPutClusterData(ctx context.Context, cd *cluster.ClusterData, previous *KVPair) (*KVPair, error)
	PutClusterData(ctx context.Context, cd *cluster.ClusterData) error
	GetClusterData(ctx context.Context) (*cluster.ClusterData, *KVPair, error)
	SetKeeperInfo(ctx context.Context, id string, ms *cluster.KeeperInfo, ttl time.Duration) error
	GetKeepersInfo(ctx context.Context) (cluster.KeepersInfo, error)
	SetSentinelInfo(ctx context.Context, si *cluster.SentinelInfo, ttl time.Duration) error
	GetSentinelsInfo(ctx context.Context) (cluster.SentinelsInfo, error)
	SetProxyInfo(ctx context.Context, pi *cluster.ProxyInfo, ttl time.Duration) error
	GetProxiesInfo(ctx context.Context) (cluster.ProxiesInfo, error)
}

type Election interface {
	// TODO(sgotti) this mimics the current docker/leadership API and the etcdv3
	// implementations adapt to it. In future it could be replaced with a better
	// api like the current one implemented by etcdclientv3/concurrency.
	RunForElection() (<-chan bool, <-chan error)
	Leader() (string, error)
	Stop()
}
