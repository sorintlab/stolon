// Package etcdv2 contains the etcd v2 store implementation.
package etcdv2

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kvtools/valkeyrie"
	"github.com/kvtools/valkeyrie/store"
	etcd "go.etcd.io/etcd/client/v2"
)

// StoreName the name of the store.
const StoreName = "etcdv2"

const (
	lockSuffix     = "___lock"
	defaultLockTTL = 20 * time.Second
)

// ErrAbortTryLock is thrown when a user stops trying to seek the lock
// by sending a signal to the stop chan,
// this is used to verify if the operation succeeded.
var ErrAbortTryLock = errors.New("lock operation aborted")

// registers etcd v2 to Valkeyrie.
func init() {
	valkeyrie.Register(StoreName, newStore)
}

// Config the etcd v2 configuration.
type Config struct {
	TLS               *tls.Config
	ConnectionTimeout time.Duration
	SyncPeriod        time.Duration
	Username          string
	Password          string
}

func newStore(ctx context.Context, endpoints []string, options valkeyrie.Config) (store.Store, error) {
	cfg, ok := options.(*Config)
	if !ok && options != nil {
		return nil, &store.InvalidConfigurationError{Store: StoreName, Config: options}
	}

	return New(ctx, endpoints, cfg)
}

// Store implements the store.Store interface.
type Store struct {
	client etcd.KeysAPI
}

// New creates a new etcd v2 client.
func New(ctx context.Context, addrs []string, options *Config) (*Store, error) {
	c, err := etcd.New(createConfig(addrs, options))
	if err != nil {
		return nil, err
	}

	s := &Store{
		client: etcd.NewKeysAPI(c),
	}

	// Periodic Cluster Sync.
	if options != nil && options.SyncPeriod != 0 {
		go func() {
			for {
				_ = c.AutoSync(ctx, options.SyncPeriod)
			}
		}()
	}

	return s, nil
}

// Get the value at "key".
// Returns the last modified index to use in conjunction to Atomic calls.
func (s *Store) Get(ctx context.Context, key string, opts *store.ReadOptions) (pair *store.KVPair, err error) {
	getOpts := &etcd.GetOptions{
		Quorum: true,
	}

	// Get options.
	if opts != nil {
		getOpts.Quorum = opts.Consistent
	}

	result, err := s.client.Get(ctx, normalize(key), getOpts)
	if err != nil {
		if keyNotFound(err) {
			return nil, store.ErrKeyNotFound
		}
		return nil, err
	}

	pair = &store.KVPair{
		Key:       key,
		Value:     []byte(result.Node.Value),
		LastIndex: result.Node.ModifiedIndex,
	}

	return pair, nil
}

// Put a value at "key".
func (s *Store) Put(ctx context.Context, key string, value []byte, opts *store.WriteOptions) error {
	setOpts := &etcd.SetOptions{}

	// Set options.
	if opts != nil {
		setOpts.Dir = opts.IsDir
		setOpts.TTL = opts.TTL
	}

	_, err := s.client.Set(ctx, normalize(key), string(value), setOpts)
	return err
}

// Delete a value at "key".
func (s *Store) Delete(ctx context.Context, key string) error {
	opts := &etcd.DeleteOptions{
		Recursive: false,
	}

	_, err := s.client.Delete(ctx, normalize(key), opts)
	if keyNotFound(err) {
		return store.ErrKeyNotFound
	}
	return err
}

// Exists checks if the key exists inside the store.
func (s *Store) Exists(ctx context.Context, key string, opts *store.ReadOptions) (bool, error) {
	_, err := s.Get(ctx, key, opts)
	if err != nil {
		if errors.Is(err, store.ErrKeyNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Watch for changes on a "key".
// It returns a channel that will receive changes or pass on errors.
// Upon creation, the current value will first be sent to the channel.
// Providing a non-nil stopCh can be used to stop watching.
func (s *Store) Watch(ctx context.Context, key string, opts *store.ReadOptions) (<-chan *store.KVPair, error) {
	wopts := &etcd.WatcherOptions{Recursive: false}
	watcher := s.client.Watcher(normalize(key), wopts)

	// watchCh is sending back events to the caller.
	watchCh := make(chan *store.KVPair)

	// Get the current value.
	pair, err := s.Get(ctx, key, opts)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(watchCh)

		// Push the current value through the channel.
		watchCh <- pair

		for {
			// Check if the watch was stopped by the caller.
			select {
			case <-ctx.Done():
				return
			default:
			}

			result, err := watcher.Next(ctx)
			if err != nil {
				return
			}

			watchCh <- &store.KVPair{
				Key:       key,
				Value:     []byte(result.Node.Value),
				LastIndex: result.Node.ModifiedIndex,
			}
		}
	}()

	return watchCh, nil
}

// WatchTree watches for changes on a "directory".
// It returns a channel that will receive changes or pass on errors.
// Upon creating a watch, the current children values will be sent to the channel.
// Providing a non-nil stopCh can be used to stop watching.
func (s *Store) WatchTree(ctx context.Context, directory string, opts *store.ReadOptions) (<-chan []*store.KVPair, error) {
	watchOpts := &etcd.WatcherOptions{Recursive: true}
	watcher := s.client.Watcher(normalize(directory), watchOpts)

	// watchCh is sending back events to the caller.
	watchCh := make(chan []*store.KVPair)

	// List current children.
	list, err := s.List(ctx, directory, opts)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(watchCh)

		// Push the current value through the channel.
		watchCh <- list

		for {
			// Check if the watch was stopped by the caller.
			select {
			case <-ctx.Done():
				return
			default:
			}

			_, err := watcher.Next(ctx)
			if err != nil {
				return
			}

			list, err = s.List(ctx, directory, opts)
			if err != nil {
				return
			}

			watchCh <- list
		}
	}()

	return watchCh, nil
}

// AtomicPut puts a value at "key" if the key has not been modified in the meantime,
// throws an error if this is the case.
func (s *Store) AtomicPut(ctx context.Context, key string, value []byte, previous *store.KVPair, opts *store.WriteOptions) (bool, *store.KVPair, error) {
	setOpts := &etcd.SetOptions{}

	setOpts.PrevExist = etcd.PrevNoExist
	if previous != nil {
		setOpts.PrevExist = etcd.PrevExist
		setOpts.PrevIndex = previous.LastIndex
		if previous.Value != nil {
			setOpts.PrevValue = string(previous.Value)
		}
	}

	if opts != nil && opts.TTL > 0 {
		setOpts.TTL = opts.TTL
	}

	meta, err := s.client.Set(ctx, normalize(key), string(value), setOpts)
	if err != nil {
		if etcdError, ok := err.(etcd.Error); ok {
			switch etcdError.Code {
			// Compare failed.
			case etcd.ErrorCodeTestFailed:
				return false, nil, store.ErrKeyModified

			// Node exists error (when PrevNoExist).
			case etcd.ErrorCodeNodeExist:
				return false, nil, store.ErrKeyExists
			}
		}
		return false, nil, err
	}

	updated := &store.KVPair{
		Key:       key,
		Value:     value,
		LastIndex: meta.Node.ModifiedIndex,
	}

	return true, updated, nil
}

// AtomicDelete deletes a value at "key" if the key has not been modified in the meantime,
// throws an error if this is the case.
func (s *Store) AtomicDelete(ctx context.Context, key string, previous *store.KVPair) (bool, error) {
	if previous == nil {
		return false, store.ErrPreviousNotSpecified
	}

	delOpts := &etcd.DeleteOptions{}

	delOpts.PrevIndex = previous.LastIndex
	if previous.Value != nil {
		delOpts.PrevValue = string(previous.Value)
	}

	_, err := s.client.Delete(ctx, normalize(key), delOpts)
	if err != nil {
		if etcdError, ok := err.(etcd.Error); ok {
			switch etcdError.Code {
			// Key Not Found.
			case etcd.ErrorCodeKeyNotFound:
				return false, store.ErrKeyNotFound

			// Compare failed.
			case etcd.ErrorCodeTestFailed:
				return false, store.ErrKeyModified
			}
		}
		return false, err
	}

	return true, nil
}

// List child nodes of a given directory.
func (s *Store) List(ctx context.Context, directory string, opts *store.ReadOptions) ([]*store.KVPair, error) {
	getOpts := &etcd.GetOptions{
		Quorum:    true,
		Recursive: true,
		Sort:      true,
	}

	// Get options.
	if opts != nil {
		getOpts.Quorum = opts.Consistent
	}

	resp, err := s.client.Get(ctx, normalize(directory), getOpts)
	if err != nil {
		if keyNotFound(err) {
			return nil, store.ErrKeyNotFound
		}
		return nil, err
	}

	var kv []*store.KVPair
	for _, n := range resp.Node.Nodes {
		if n.Key == directory {
			continue
		}

		// Etcd v2 seems to stop listing child keys at directories even with the "Recursive" option.
		// If the child is a directory, we call `List` recursively to go through the whole set.
		if n.Dir {
			pairs, err := s.List(ctx, n.Key, opts)
			if err != nil {
				return nil, err
			}
			kv = append(kv, pairs...)
		}

		// Filter out etcd mutex side keys with `___lock` suffix.
		if strings.Contains(n.Key, lockSuffix) {
			continue
		}

		kv = append(kv, &store.KVPair{
			Key:       n.Key,
			Value:     []byte(n.Value),
			LastIndex: n.ModifiedIndex,
		})
	}
	return kv, nil
}

// DeleteTree deletes a range of keys under a given directory.
func (s *Store) DeleteTree(ctx context.Context, directory string) error {
	delOpts := &etcd.DeleteOptions{
		Recursive: true,
	}

	_, err := s.client.Delete(ctx, normalize(directory), delOpts)
	if keyNotFound(err) {
		return store.ErrKeyNotFound
	}
	return err
}

// NewLock returns a handle to a lock struct
// which can be used to provide mutual exclusion on a key.
func (s *Store) NewLock(_ context.Context, key string, opts *store.LockOptions) (lock store.Locker, err error) {
	ttl := defaultLockTTL
	renewCh := make(chan struct{})

	// Apply options on Lock.
	var value string
	if opts != nil {
		if opts.Value != nil {
			value = string(opts.Value)
		}

		if opts.TTL != 0 {
			ttl = opts.TTL
		}

		if opts.RenewLock != nil {
			renewCh = opts.RenewLock
		}
	}

	// Create lock object.
	lock = &etcdLock{
		client:    s.client,
		stopRenew: renewCh,
		mutexKey:  normalize(key + lockSuffix),
		writeKey:  normalize(key),
		value:     value,
		ttl:       ttl,
	}

	return lock, nil
}

// Close closes the client connection.
func (s *Store) Close() error { return nil }

type etcdLock struct {
	lock   sync.Mutex
	client etcd.KeysAPI

	stopLock  chan struct{}
	stopRenew chan struct{}

	mutexKey string
	writeKey string
	value    string
	last     *etcd.Response
	ttl      time.Duration
}

// Lock attempts to acquire the lock and blocks while doing so.
// It returns a channel that is closed if our lock is lost or if an error occurs.
func (l *etcdLock) Lock(ctx context.Context) (<-chan struct{}, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	// Lock holder channel.
	lockHeld := make(chan struct{})
	stopLocking := l.stopRenew

	setOpts := &etcd.SetOptions{TTL: l.ttl}

	for {
		setOpts.PrevExist = etcd.PrevNoExist

		resp, err := l.client.Set(ctx, l.mutexKey, "", setOpts)
		if err != nil {
			if etcdError, ok := err.(etcd.Error); ok {
				if etcdError.Code != etcd.ErrorCodeNodeExist {
					return nil, err
				}
				setOpts.PrevIndex = ^uint64(0)
			}
		} else {
			setOpts.PrevIndex = resp.Node.ModifiedIndex
		}

		setOpts.PrevExist = etcd.PrevExist

		l.last, err = l.client.Set(ctx, l.mutexKey, "", setOpts)
		if err == nil {
			// Leader section.
			l.stopLock = stopLocking
			go l.holdLock(ctx, l.mutexKey, lockHeld, stopLocking)

			// We are holding the lock, set the write key.
			_, err = l.client.Set(ctx, l.writeKey, l.value, nil)
			if err != nil {
				return nil, err
			}

			break
		}

		// If this is a legitimate error, return.
		if etcdError, ok := err.(etcd.Error); ok {
			if etcdError.Code != etcd.ErrorCodeTestFailed {
				return nil, err
			}
		}

		// Seeker section.
		errorCh := make(chan error)
		chWStop := make(chan bool)
		free := make(chan bool)

		go l.waitLock(ctx, l.mutexKey, errorCh, chWStop, free)

		// Wait for the key to be available or
		// for a signal to stop trying to lock the key.
		select {
		case <-free:
		case err := <-errorCh:
			return nil, err
		case <-ctx.Done():
			return nil, ErrAbortTryLock
		}

		// Delete or Expire event occurred.
		// Retry.
	}

	return lockHeld, nil
}

// holdLock holds the lock
// as long as we can update the key ttl periodically
// until we receive an explicit stop signal from the Unlock method.
func (l *etcdLock) holdLock(ctx context.Context, key string, lockHeld chan struct{}, stopLocking <-chan struct{}) {
	defer close(lockHeld)

	update := time.NewTicker(l.ttl / 3)
	defer update.Stop()

	var err error
	setOpts := &etcd.SetOptions{TTL: l.ttl}

	for {
		select {
		case <-update.C:
			setOpts.PrevIndex = l.last.Node.ModifiedIndex
			l.last, err = l.client.Set(ctx, key, "", setOpts)
			if err != nil {
				return
			}

		case <-stopLocking:
			return
		}
	}
}

// waitLock simply waits for the key to be available for creation.
func (l *etcdLock) waitLock(ctx context.Context, key string, errorCh chan error, _ chan bool, free chan<- bool) {
	opts := &etcd.WatcherOptions{Recursive: false}
	watcher := l.client.Watcher(key, opts)

	for {
		event, err := watcher.Next(ctx)
		if err != nil {
			errorCh <- err
			return
		}
		if event.Action == "delete" || event.Action == "compareAndDelete" || event.Action == "expire" {
			free <- true
			return
		}
	}
}

// Unlock the "key".
// Calling unlock while not holding the lock will throw an error.
func (l *etcdLock) Unlock(ctx context.Context) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.stopLock != nil {
		l.stopLock <- struct{}{}
	}

	if l.last != nil {
		delOpts := &etcd.DeleteOptions{
			PrevIndex: l.last.Node.ModifiedIndex,
		}
		_, err := l.client.Delete(ctx, l.mutexKey, delOpts)
		if err != nil {
			return err
		}
	}

	return nil
}

// keyNotFound checks on the error returned by the KeysAPI
// to verify if the key exists in the store or not.
func keyNotFound(err error) bool {
	if err == nil {
		return false
	}

	etcdError, ok := err.(etcd.Error)
	if ok && etcdError.Code == etcd.ErrorCodeKeyNotFound ||
		etcdError.Code == etcd.ErrorCodeNotFile ||
		etcdError.Code == etcd.ErrorCodeNotDir {
		return true
	}
	return false
}

// normalize the key for usage in Etcd.
func normalize(key string) string {
	return strings.TrimPrefix(key, "/")
}

func createConfig(addrs []string, options *Config) etcd.Config {
	cfg := etcd.Config{
		Endpoints:               store.CreateEndpoints(addrs, "http"),
		Transport:               etcd.DefaultTransport,
		HeaderTimeoutPerRequest: 3 * time.Second,
	}

	if options == nil {
		return cfg
	}

	if options.TLS != nil {
		entries := store.CreateEndpoints(addrs, "https")
		cfg.Endpoints = entries

		// Set transport.
		cfg.Transport = &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig:     options.TLS,
		}
	}

	if options.ConnectionTimeout != 0 {
		cfg.HeaderTimeoutPerRequest = options.ConnectionTimeout
	}

	if options.Username != "" {
		cfg.Username = options.Username
		cfg.Password = options.Password
	}

	return cfg
}
