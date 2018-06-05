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

	"github.com/docker/leadership"
	libkvstore "github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	"github.com/docker/libkv/store/etcd"
)

func init() {
	etcd.Register()
	consul.Register()
}

func fromLibKVStoreErr(err error) error {
	switch err {
	case libkvstore.ErrKeyNotFound:
		return ErrKeyNotFound
	case libkvstore.ErrKeyModified:
		return ErrKeyModified
	}
	return err
}

type libKVStore struct {
	store libkvstore.Store
}

func (s *libKVStore) Put(ctx context.Context, key string, value []byte, options *WriteOptions) error {
	var libkvOptions *libkvstore.WriteOptions
	if options != nil {
		libkvOptions = &libkvstore.WriteOptions{TTL: options.TTL}
	}
	err := s.store.Put(key, value, libkvOptions)
	return fromLibKVStoreErr(err)
}

func (s *libKVStore) Get(ctx context.Context, key string) (*KVPair, error) {
	pair, err := s.store.Get(key)
	if err != nil {
		return nil, fromLibKVStoreErr(err)
	}
	return &KVPair{Key: pair.Key, Value: pair.Value, LastIndex: pair.LastIndex}, nil
}

func (s *libKVStore) List(ctx context.Context, directory string) ([]*KVPair, error) {
	pairs, err := s.store.List(directory)
	if err != nil {
		return nil, fromLibKVStoreErr(err)
	}
	kvPairs := make([]*KVPair, len(pairs))
	for i, p := range pairs {
		kvPairs[i] = &KVPair{Key: p.Key, Value: p.Value, LastIndex: p.LastIndex}
	}
	return kvPairs, nil
}

func (s *libKVStore) AtomicPut(ctx context.Context, key string, value []byte, previous *KVPair, options *WriteOptions) (*KVPair, error) {
	var libkvPrevious *libkvstore.KVPair
	if previous != nil {
		libkvPrevious = &libkvstore.KVPair{Key: previous.Key, LastIndex: previous.LastIndex}
	}
	var libkvOptions *libkvstore.WriteOptions
	if options != nil {
		libkvOptions = &libkvstore.WriteOptions{TTL: options.TTL}
	}
	_, pair, err := s.store.AtomicPut(key, value, libkvPrevious, libkvOptions)
	if err != nil {
		return nil, fromLibKVStoreErr(err)
	}
	return &KVPair{Key: pair.Key, Value: pair.Value, LastIndex: pair.LastIndex}, nil
}

func (s *libKVStore) Delete(ctx context.Context, key string) error {
	return fromLibKVStoreErr(s.store.Delete(key))
}

func (s *libKVStore) Close() error {
	s.store.Close()
	return nil
}

type libkvElection struct {
	store     *libKVStore
	path      string
	candidate *leadership.Candidate
}

func (e *libkvElection) RunForElection() (<-chan bool, <-chan error) {
	return e.candidate.RunForElection()
}

func (e *libkvElection) Stop() {
	e.candidate.Stop()
}

func (e *libkvElection) Leader() (string, error) {
	pair, err := e.store.Get(context.TODO(), e.path)
	if err != nil {
		if err != ErrKeyNotFound {
			return "", err
		}
		return "", nil
	}
	return string(pair.Value), nil
}
