// Package consul contains the Consul store implementation.
package consul

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/kvtools/valkeyrie"
	"github.com/kvtools/valkeyrie/store"
)

// StoreName the name of the store.
const StoreName = "consul"

const (
	// DefaultWatchWaitTime is how long we block for at a time to check if the watched key has changed.
	// This affects the minimum time it takes to cancel a watch.
	DefaultWatchWaitTime = 15 * time.Second

	// RenewSessionRetryMax is the number of time we should try to renew the session before giving up and throwing an error.
	RenewSessionRetryMax = 5

	// MaxSessionDestroyAttempts is the maximum times we will try
	// to explicitly destroy the session attached to a lock after
	// the connectivity to the store has been lost.
	MaxSessionDestroyAttempts = 5

	// defaultLockTTL is the default ttl for the consul lock.
	defaultLockTTL = 20 * time.Second
)

var (
	// ErrMultipleEndpointsUnsupported is thrown when there are multiple endpoints specified for Consul.
	ErrMultipleEndpointsUnsupported = errors.New("consul does not support multiple endpoints")

	// ErrSessionRenew is thrown when the session can't be renewed because the Consul version does not support sessions.
	ErrSessionRenew = errors.New("cannot set or renew session for ttl, unable to operate on sessions")
)

// registers Consul to Valkeyrie.
func init() {
	valkeyrie.Register(StoreName, newStore)
}

// Config the Consul configuration.
type Config struct {
	TLS               *tls.Config
	ConnectionTimeout time.Duration
	Token             string
	Namespace         string
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
	client *api.Client
}

// New creates a new Consul client.
func New(_ context.Context, endpoints []string, options *Config) (*Store, error) {
	if len(endpoints) > 1 {
		return nil, ErrMultipleEndpointsUnsupported
	}

	config := createConfig(endpoints, options)

	// Creates a new client.
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &Store{client: client}, nil
}

// Get the value at "key".
// Returns the last modified index to use in conjunction to CAS calls.
func (s *Store) Get(_ context.Context, key string, opts *store.ReadOptions) (*store.KVPair, error) {
	options := &api.QueryOptions{
		AllowStale:        false,
		RequireConsistent: true,
	}

	// Get options.
	if opts != nil {
		options.RequireConsistent = opts.Consistent
	}

	pair, meta, err := s.client.KV().Get(normalize(key), options)
	if err != nil {
		return nil, err
	}

	// If pair is nil then the key does not exist.
	if pair == nil {
		return nil, store.ErrKeyNotFound
	}

	return &store.KVPair{Key: pair.Key, Value: pair.Value, LastIndex: meta.LastIndex}, nil
}

// Put a value at "key".
func (s *Store) Put(_ context.Context, key string, value []byte, opts *store.WriteOptions) error {
	p := &api.KVPair{
		Key:   normalize(key),
		Value: value,
		Flags: api.LockFlagValue,
	}

	if opts != nil && opts.TTL > 0 {
		// Create or renew a session holding a TTL.
		// Operations on sessions are not deterministic: creating or renewing a session can fail.
		for retry := 1; retry <= RenewSessionRetryMax; retry++ {
			err := s.renewSession(p, opts.TTL)
			if err == nil {
				break
			}
			if retry == RenewSessionRetryMax {
				return ErrSessionRenew
			}
		}
	}

	_, err := s.client.KV().Put(p, nil)
	return err
}

// Delete a value at "key".
func (s *Store) Delete(ctx context.Context, key string) error {
	if _, err := s.Get(ctx, key, nil); err != nil {
		return err
	}

	_, err := s.client.KV().Delete(normalize(key), nil)
	return err
}

// Exists checks that the key exists inside the store.
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

// List child nodes of a given directory.
func (s *Store) List(_ context.Context, directory string, opts *store.ReadOptions) ([]*store.KVPair, error) {
	options := &api.QueryOptions{
		AllowStale:        false,
		RequireConsistent: true,
	}

	if opts != nil && !opts.Consistent {
		options.AllowStale = true
		options.RequireConsistent = false
	}

	pairs, _, err := s.client.KV().List(normalize(directory), options)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, store.ErrKeyNotFound
	}

	var kv []*store.KVPair

	for _, pair := range pairs {
		if pair.Key == directory {
			continue
		}
		kv = append(kv, &store.KVPair{
			Key:       pair.Key,
			Value:     pair.Value,
			LastIndex: pair.ModifyIndex,
		})
	}

	return kv, nil
}

// DeleteTree deletes a range of keys under a given directory.
func (s *Store) DeleteTree(ctx context.Context, directory string) error {
	if _, err := s.List(ctx, directory, nil); err != nil {
		return err
	}

	_, err := s.client.KV().DeleteTree(normalize(directory), nil)
	return err
}

// Watch for changes on a "key".
// It returns a channel that will receive changes or pass on errors.
// Upon creation, the current value will first be sent to the channel.
// Providing a non-nil stopCh can be used to stop watching.
func (s *Store) Watch(ctx context.Context, key string, _ *store.ReadOptions) (<-chan *store.KVPair, error) {
	kv := s.client.KV()
	watchCh := make(chan *store.KVPair)

	go func() {
		defer close(watchCh)

		// Use a wait time in order to check if we should quit from time to time.
		opts := &api.QueryOptions{WaitTime: DefaultWatchWaitTime}

		for {
			// Check if we should quit.
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Get the key.
			pair, meta, err := kv.Get(key, opts)
			if err != nil {
				return
			}

			// If LastIndex didn't change then it means `Get` returned
			// because of the WaitTime and the key didn't change.
			if opts.WaitIndex == meta.LastIndex {
				continue
			}
			opts.WaitIndex = meta.LastIndex

			// Return the value to the channel.
			if pair != nil {
				watchCh <- &store.KVPair{
					Key:       pair.Key,
					Value:     pair.Value,
					LastIndex: pair.ModifyIndex,
				}
			}
		}
	}()

	return watchCh, nil
}

// WatchTree watches for changes on a "directory".
// It returns a channel that will receive changes or pass on errors.
// Upon creating a watch, the current children values will be sent to the channel.
// Providing a non-nil stopCh can be used to stop watching.
func (s *Store) WatchTree(ctx context.Context, directory string, _ *store.ReadOptions) (<-chan []*store.KVPair, error) {
	kv := s.client.KV()
	watchCh := make(chan []*store.KVPair)

	go func() {
		defer close(watchCh)

		// Use a wait time in order to check if we should quit from time to time.
		opts := &api.QueryOptions{WaitTime: DefaultWatchWaitTime}
		for {
			// Check if we should quit.
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Get all the children.
			pairs, meta, err := kv.List(directory, opts)
			if err != nil {
				return
			}

			// If LastIndex didn't change then it means `Get` returned
			// because of the WaitTime and the child keys didn't change.
			if opts.WaitIndex == meta.LastIndex {
				continue
			}
			opts.WaitIndex = meta.LastIndex

			// Return children KV pairs to the channel.
			var kvPairs []*store.KVPair
			for _, pair := range pairs {
				if pair.Key == directory {
					continue
				}
				kvPairs = append(kvPairs, &store.KVPair{
					Key:       pair.Key,
					Value:     pair.Value,
					LastIndex: pair.ModifyIndex,
				})
			}
			watchCh <- kvPairs
		}
	}()

	return watchCh, nil
}

// NewLock returns a handle to a lock struct which can be used to provide mutual exclusion on a key.
func (s *Store) NewLock(ctx context.Context, key string, opts *store.LockOptions) (store.Locker, error) {
	ttl := defaultLockTTL

	lockOpts := &api.LockOptions{
		Key: normalize(key),
	}

	if opts != nil {
		// Set optional TTL on Lock.
		if opts.TTL != 0 {
			ttl = opts.TTL
		}
		// Set optional value on Lock.
		if opts.Value != nil {
			lockOpts.Value = opts.Value
		}
	}

	entry := &api.SessionEntry{
		Behavior:  api.SessionBehaviorRelease, // Release the lock when the session expires.
		TTL:       (ttl / 2).String(),         // Consul multiplies the TTL by 2x.
		LockDelay: 1 * time.Millisecond,       // Virtually disable lock delay.
	}

	q := (&api.WriteOptions{}).WithContext(ctx)

	// Create the key session.
	session, _, err := s.client.Session().Create(entry, q)
	if err != nil {
		return nil, err
	}

	lock := &consulLock{}

	// Place the session and renew chan on lock.
	lockOpts.Session = session
	if opts != nil {
		lock.renewCh = opts.RenewLock
	}

	l, err := s.client.LockOpts(lockOpts)
	if err != nil {
		return nil, err
	}

	// Renew the session ttl lock periodically.
	if opts != nil {
		s.renewLockSession(entry.TTL, session, opts.RenewLock, q)
	}

	lock.lock = l

	return lock, nil
}

// renewLockSession is used to renew a session Lock,
// it takes a stopRenew chan which is used to explicitly stop the session renew process.
// The renewal routine never stops until a signal is sent to this channel.
// If deleting the session fails because the connection to the store is lost,
// it keeps trying to delete the session periodically until it can contact the store,
// this ensures that the lock is not maintained indefinitely which ensures liveness
// over safety for the lock when the store becomes unavailable.
func (s *Store) renewLockSession(initialTTL string, id string, stopRenew chan struct{}, q *api.WriteOptions) {
	ttl, err := time.ParseDuration(initialTTL)
	if err != nil {
		return
	}

	sessionDestroyAttempts := 0

	go func() {
		for {
			select {
			case <-time.After(ttl / 2):
				entry, _, err := s.client.Session().Renew(id, q)
				if err != nil {
					// If an error occurs,
					// continue until the session gets destroyed explicitly or the session ttl times out.
					continue
				}
				if entry == nil {
					return
				}

				// Handle the server updating the TTL.
				ttl, _ = time.ParseDuration(entry.TTL)

			case <-stopRenew:
				// Attempt a session destroy.
				_, err := s.client.Session().Destroy(id, q)
				if err == nil {
					return
				}

				// We cannot destroy the session because the store is unavailable,
				// wait for the session renew period.
				// Give up after 'MaxSessionDestroyAttempts'.
				sessionDestroyAttempts++

				if sessionDestroyAttempts >= MaxSessionDestroyAttempts {
					return
				}

				time.Sleep(ttl / 2)
			}
		}
	}()
}

// AtomicPut puts a value at "key" if the key has not been modified in the meantime,
// throws an error if this is the case.
func (s *Store) AtomicPut(ctx context.Context, key string, value []byte, previous *store.KVPair, _ *store.WriteOptions) (bool, *store.KVPair, error) {
	p := &api.KVPair{Key: normalize(key), Value: value, Flags: api.LockFlagValue}

	// Consul interprets ModifyIndex = 0 as new key.
	p.ModifyIndex = 0

	if previous != nil {
		p.ModifyIndex = previous.LastIndex
	}

	ok, _, err := s.client.KV().CAS(p, nil)
	if err != nil {
		return false, nil, err
	}
	if !ok {
		if previous == nil {
			return false, nil, store.ErrKeyExists
		}
		return false, nil, store.ErrKeyModified
	}

	pair, err := s.Get(ctx, key, nil)
	if err != nil {
		return false, nil, err
	}

	return true, pair, nil
}

// AtomicDelete deletes a value at "key" if the key has not been modified in the meantime,
// throws an error if this is the case.
func (s *Store) AtomicDelete(ctx context.Context, key string, previous *store.KVPair) (bool, error) {
	if previous == nil {
		return false, store.ErrPreviousNotSpecified
	}

	p := &api.KVPair{Key: normalize(key), ModifyIndex: previous.LastIndex, Flags: api.LockFlagValue}

	// Extra Get operation to check on the key.
	_, err := s.Get(ctx, key, nil)
	if errors.Is(err, store.ErrKeyNotFound) {
		return false, err
	}

	if work, _, err := s.client.KV().DeleteCAS(p, nil); err != nil {
		return false, err
	} else if !work {
		return false, store.ErrKeyModified
	}

	return true, nil
}

// Close closes the client connection.
func (s *Store) Close() error { return nil }

func (s *Store) renewSession(pair *api.KVPair, ttl time.Duration) error {
	// Check if there is any previous session with an active TTL.
	session, err := s.getActiveSession(pair.Key)
	if err != nil {
		return err
	}

	if session == "" {
		entry := &api.SessionEntry{
			Behavior:  api.SessionBehaviorDelete, // Delete the key when the session expires.
			TTL:       (ttl / 2).String(),        // Consul multiplies the TTL by 2x.
			LockDelay: 1 * time.Millisecond,      // Virtually disable lock delay.
		}

		// Create the key session.
		session, _, err = s.client.Session().Create(entry, nil)
		if err != nil {
			return err
		}

		lockOpts := &api.LockOptions{
			Key:     pair.Key,
			Session: session,
		}

		// Lock and ignore if lock is held.
		// It's just a placeholder for the ephemeral behavior.
		lock, _ := s.client.LockOpts(lockOpts)
		if lock != nil {
			_, _ = lock.Lock(nil)
		}
	}

	_, _, err = s.client.Session().Renew(session, nil)
	return err
}

// getActiveSession checks if the key already has a session attached.
func (s *Store) getActiveSession(key string) (string, error) {
	pair, _, err := s.client.KV().Get(key, nil)
	if err != nil {
		return "", err
	}

	if pair == nil || pair.Session == "" {
		return "", nil
	}

	return pair.Session, nil
}

func createConfig(endpoints []string, options *Config) *api.Config {
	config := api.DefaultConfig()
	config.HttpClient = http.DefaultClient
	config.Address = endpoints[0]

	if options != nil {
		if options.TLS != nil {
			config.HttpClient.Transport = &http.Transport{
				TLSClientConfig: options.TLS,
			}
			config.Scheme = "https"
		}

		if options.ConnectionTimeout != 0 {
			config.WaitTime = options.ConnectionTimeout
		}

		if options.Token != "" {
			config.Token = options.Token
		}

		if options.Namespace != "" {
			config.Namespace = options.Namespace
		}
	}

	return config
}

// normalize the key for usage in Consul.
func normalize(key string) string {
	return strings.TrimPrefix(key, "/")
}

type consulLock struct {
	lock    *api.Lock
	renewCh chan struct{}
}

// Lock attempts to acquire the lock and blocks while doing so.
// It returns a channel that is closed if our lock is lost or if an error occurs.
func (l *consulLock) Lock(ctx context.Context) (<-chan struct{}, error) {
	return l.lock.Lock(ctx.Done())
}

// Unlock the "key".
// Calling unlock while not holding the lock will throw an error.
func (l *consulLock) Unlock(_ context.Context) error {
	if l.renewCh != nil {
		close(l.renewCh)
	}

	return l.lock.Unlock()
}
