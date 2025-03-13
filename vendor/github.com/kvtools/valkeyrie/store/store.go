// Package store contains KV store backends.
package store

import (
	"context"
	"time"
)

// Store represents the backend K/V storage.
// Each store should support every call listed here.
// Or it couldn't be implemented as a K/V backend for valkeyrie.
type Store interface {
	// Put a value at the specified key.
	Put(ctx context.Context, key string, value []byte, opts *WriteOptions) error

	// Get a value given its key.
	Get(ctx context.Context, key string, opts *ReadOptions) (*KVPair, error)

	// Delete the value at the specified key.
	Delete(ctx context.Context, key string) error

	// Exists Verify if a Key exists in the store.
	Exists(ctx context.Context, key string, opts *ReadOptions) (bool, error)

	// Watch for changes on a key.
	Watch(ctx context.Context, key string, opts *ReadOptions) (<-chan *KVPair, error)

	// WatchTree watches for changes on child nodes under a given directory.
	WatchTree(ctx context.Context, directory string, opts *ReadOptions) (<-chan []*KVPair, error)

	// NewLock creates a lock for a given key.
	// The returned Locker is not held and must be acquired with `.Lock`.
	// The Value is optional.
	NewLock(ctx context.Context, key string, opts *LockOptions) (Locker, error)

	// List the content of a given prefix.
	List(ctx context.Context, directory string, opts *ReadOptions) ([]*KVPair, error)

	// DeleteTree deletes a range of keys under a given directory.
	DeleteTree(ctx context.Context, directory string) error

	// AtomicPut Atomic CAS operation on a single value.
	// Pass previous = nil to create a new key.
	AtomicPut(ctx context.Context, key string, value []byte, previous *KVPair, opts *WriteOptions) (bool, *KVPair, error)

	// AtomicDelete Atomic delete of a single value.
	AtomicDelete(ctx context.Context, key string, previous *KVPair) (bool, error)

	// Close the store connection.
	Close() error
}

// KVPair represents {Key, Value, LastIndex} tuple.
type KVPair struct {
	Key       string
	Value     []byte
	LastIndex uint64
}

// WriteOptions contains optional request parameters.
type WriteOptions struct {
	IsDir bool
	TTL   time.Duration

	// If true, the client will keep the lease alive in the background
	// for stores that are allowing it.
	KeepAlive bool
}

// ReadOptions contains optional request parameters.
type ReadOptions struct {
	// Consistent defines if the behavior of a Get operation is linearizable or not.
	// Linearizability allows us to 'see' objects based on a real-time total order
	// as opposed to an arbitrary order or with stale values ('inconsistent' scenario).
	Consistent bool
}

// LockOptions contains optional request parameters.
type LockOptions struct {
	Value          []byte        // Optional, value to associate with the lock.
	TTL            time.Duration // Optional, expiration ttl associated with the lock.
	RenewLock      chan struct{} // Optional, chan used to control and stop the session ttl renewal for the lock.
	DeleteOnUnlock bool          // If true, the value will be deleted when the lock is unlocked or expires.
}

// Locker provides locking mechanism on top of the store.
// Similar to sync.Locker except it may return errors.
type Locker interface {
	Lock(ctx context.Context) (<-chan struct{}, error)
	Unlock(ctx context.Context) error
}
