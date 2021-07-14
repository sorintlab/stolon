// Copyright 2017 Sorint.lab
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
	"time"

	etcdclientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
)

func fromEtcV3Error(err error) error {
	switch err {
	case rpctypes.ErrKeyNotFound:
		return ErrKeyNotFound
	case concurrency.ErrElectionNoLeader:
		return ErrElectionNoLeader
	}
	return err
}

type etcdV3Store struct {
	c              *etcdclientv3.Client
	requestTimeout time.Duration
}

func (s *etcdV3Store) Put(pctx context.Context, key string, value []byte, options *WriteOptions) error {
	etcdv3Options := []etcdclientv3.OpOption{}
	if options != nil {
		if options.TTL > 0 {
			ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
			lease, err := s.c.Grant(ctx, int64(options.TTL.Seconds()))
			cancel()
			if err != nil {
				return err
			}
			etcdv3Options = append(etcdv3Options, etcdclientv3.WithLease(lease.ID))
		}
	}
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	_, err := s.c.Put(ctx, key, string(value), etcdv3Options...)
	cancel()
	return fromLibKVStoreErr(err)
}

func (s *etcdV3Store) Get(pctx context.Context, key string) (*KVPair, error) {
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	resp, err := s.c.Get(ctx, key)
	cancel()
	if err != nil {
		return nil, fromEtcV3Error(err)
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrKeyNotFound
	}
	kv := resp.Kvs[0]
	return &KVPair{Key: string(kv.Key), Value: kv.Value, LastIndex: uint64(kv.ModRevision)}, nil
}

func (s *etcdV3Store) List(pctx context.Context, directory string) ([]*KVPair, error) {
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	resp, err := s.c.Get(ctx, directory, etcdclientv3.WithPrefix())
	cancel()
	if err != nil {
		return nil, fromEtcV3Error(err)
	}
	kvPairs := make([]*KVPair, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		kvPairs[i] = &KVPair{Key: string(kv.Key), Value: kv.Value, LastIndex: uint64(kv.ModRevision)}
	}
	return kvPairs, nil
}

func (s *etcdV3Store) AtomicPut(pctx context.Context, key string, value []byte, previous *KVPair, options *WriteOptions) (*KVPair, error) {
	etcdv3Options := []etcdclientv3.OpOption{}
	if options != nil {
		if options.TTL > 0 {
			ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
			lease, err := s.c.Grant(ctx, int64(options.TTL))
			cancel()
			if err != nil {
				return nil, err
			}
			etcdv3Options = append(etcdv3Options, etcdclientv3.WithLease(lease.ID))
		}
	}
	var cmp etcdclientv3.Cmp
	if previous != nil {
		cmp = etcdclientv3.Compare(etcdclientv3.ModRevision(key), "=", int64(previous.LastIndex))
	} else {
		// key doens't exists
		cmp = etcdclientv3.Compare(etcdclientv3.CreateRevision(key), "=", 0)
	}
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	txn := s.c.Txn(ctx).If(cmp)
	txn = txn.Then(etcdclientv3.OpPut(key, string(value), etcdv3Options...))
	tresp, err := txn.Commit()
	cancel()
	if err != nil {
		return nil, fromEtcV3Error(err)
	}
	if !tresp.Succeeded {
		return nil, ErrKeyModified
	}
	revision := tresp.Responses[0].GetResponsePut().Header.Revision
	return &KVPair{Key: key, Value: value, LastIndex: uint64(revision)}, nil
}

func (s *etcdV3Store) Delete(pctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	_, err := s.c.Delete(ctx, key)
	cancel()
	return fromEtcV3Error(err)
}

func (s *etcdV3Store) Close() error {
	return s.c.Close()
}

type etcdv3Election struct {
	c            *etcdclientv3.Client
	path         string
	candidateUID string
	ttl          time.Duration

	requestTimeout time.Duration

	running bool

	electedCh chan bool
	errCh     chan error

	ctx    context.Context
	cancel context.CancelFunc
}

func (e *etcdv3Election) RunForElection() (<-chan bool, <-chan error) {
	if e.running {
		panic("already running")
	}

	e.electedCh = make(chan bool)
	e.errCh = make(chan error)
	e.ctx, e.cancel = context.WithCancel(context.Background())

	e.running = true
	go e.campaign()

	return e.electedCh, e.errCh
}

func (e *etcdv3Election) Stop() {
	if !e.running {
		panic("not running")
	}
	e.cancel()
	e.running = false
}

func (e *etcdv3Election) Leader() (string, error) {
	s, err := concurrency.NewSession(e.c, concurrency.WithTTL(int(e.ttl.Seconds())))
	if err != nil {
		return "", fromEtcV3Error(err)
	}
	defer s.Close()

	etcdElection := concurrency.NewElection(s, e.path)

	ctx, cancel := context.WithTimeout(context.Background(), e.requestTimeout)
	resp, err := etcdElection.Leader(ctx)
	cancel()
	if err != nil {
		return "", fromEtcV3Error(err)
	}
	leader := string(resp.Kvs[0].Value)

	return leader, nil
}

func (e *etcdv3Election) campaign() {
	defer close(e.electedCh)
	defer close(e.errCh)

	for {
		e.electedCh <- false
		s, err := concurrency.NewSession(e.c, concurrency.WithTTL(int(e.ttl.Seconds())), concurrency.WithContext(e.ctx))
		if err != nil {
			e.errCh <- err
			return
		}

		etcdElection := concurrency.NewElection(s, e.path)
		if err = etcdElection.Campaign(e.ctx, e.candidateUID); err != nil {
			e.errCh <- err
			return
		}

		e.electedCh <- true

		select {
		case <-e.ctx.Done():
			return
		case <-s.Done():
			e.electedCh <- false
		}
	}
}
