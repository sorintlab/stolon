// Copyright 2014 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lease

import (
	"encoding/json"
	"path"
	"time"

	etcd "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/etcd/client"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
)

const (
	leasePrefix = "lease"
)

type etcdLeaseMetadata struct {
	MachineID string
	Version   int
}

// etcdLease implements the Lease interface
type etcdLease struct {
	mgr  *etcdLeaseManager
	key  string
	meta etcdLeaseMetadata
	idx  uint64
	ttl  time.Duration
}

func (l *etcdLease) Release() error {
	opts := &etcd.DeleteOptions{
		PrevIndex: l.idx,
	}
	_, err := l.mgr.kAPI.Delete(l.mgr.ctx(), l.key, opts)
	return err
}

func (l *etcdLease) Renew(period time.Duration) error {
	val, err := serializeLeaseMetadata(l.meta.MachineID, l.meta.Version)
	opts := &etcd.SetOptions{
		PrevIndex: l.idx,
		TTL:       period,
	}
	resp, err := l.mgr.kAPI.Set(l.mgr.ctx(), l.key, val, opts)
	if err != nil {
		return err
	}

	renewed := l.mgr.leaseFromResponse(resp)
	*l = *renewed

	return nil
}

func (l *etcdLease) MachineID() string {
	return l.meta.MachineID
}

func (l *etcdLease) Version() int {
	return l.meta.Version
}

func (l *etcdLease) Index() uint64 {
	return l.idx
}

func (l *etcdLease) TimeRemaining() time.Duration {
	return l.ttl
}

func serializeLeaseMetadata(machID string, ver int) (string, error) {
	meta := etcdLeaseMetadata{
		MachineID: machID,
		Version:   ver,
	}

	b, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

type etcdLeaseManager struct {
	kAPI       etcd.KeysAPI
	keyPrefix  string
	reqTimeout time.Duration
}

func NewEtcdLeaseManager(kAPI etcd.KeysAPI, keyPrefix string, reqTimeout time.Duration) *etcdLeaseManager {
	return &etcdLeaseManager{kAPI: kAPI, keyPrefix: keyPrefix, reqTimeout: reqTimeout}
}

func (r *etcdLeaseManager) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), r.reqTimeout)
	return ctx
}

func (r *etcdLeaseManager) leasePath(name string) string {
	return path.Join(r.keyPrefix, leasePrefix, name)
}

func (r *etcdLeaseManager) GetLease(name string) (Lease, error) {
	key := r.leasePath(name)
	resp, err := r.kAPI.Get(r.ctx(), key, nil)
	if err != nil {
		if isEtcdError(err, etcd.ErrorCodeKeyNotFound) {
			err = nil
		}
		return nil, err
	}

	l := r.leaseFromResponse(resp)
	return l, nil
}

func (r *etcdLeaseManager) StealLease(name, machID string, ver int, period time.Duration, idx uint64) (Lease, error) {
	val, err := serializeLeaseMetadata(machID, ver)
	if err != nil {
		return nil, err
	}

	key := r.leasePath(name)
	opts := &etcd.SetOptions{
		PrevIndex: idx,
		TTL:       period,
	}
	resp, err := r.kAPI.Set(r.ctx(), key, val, opts)
	if err != nil {
		if isEtcdError(err, etcd.ErrorCodeNodeExist) {
			err = nil
		}
		return nil, err
	}

	l := r.leaseFromResponse(resp)
	return l, nil
}

func (r *etcdLeaseManager) AcquireLease(name string, machID string, ver int, period time.Duration) (Lease, error) {
	val, err := serializeLeaseMetadata(machID, ver)
	if err != nil {
		return nil, err
	}

	key := r.leasePath(name)
	opts := &etcd.SetOptions{
		TTL:       period,
		PrevExist: etcd.PrevNoExist,
	}

	resp, err := r.kAPI.Set(r.ctx(), key, val, opts)
	if err != nil {
		if isEtcdError(err, etcd.ErrorCodeNodeExist) {
			err = nil
		}
		return nil, err
	}

	l := r.leaseFromResponse(resp)
	return l, nil
}

func (r *etcdLeaseManager) leaseFromResponse(res *etcd.Response) *etcdLease {
	l := &etcdLease{
		mgr: r,
		key: res.Node.Key,
		idx: res.Node.ModifiedIndex,
		ttl: res.Node.TTLDuration(),
	}

	err := json.Unmarshal([]byte(res.Node.Value), &l.meta)

	// fall back to using the entire value as the MachineID for
	// backwards-compatibility with engines that are not aware
	// of this versioning mechanism
	if err != nil {
		l.meta = etcdLeaseMetadata{
			MachineID: res.Node.Value,
			Version:   0,
		}
	}

	return l
}

func isEtcdError(err error, code int) bool {
	eerr, ok := err.(etcd.Error)
	return ok && eerr.Code == code
}
