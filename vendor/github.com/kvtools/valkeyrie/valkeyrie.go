// Package valkeyrie Distributed Key/Value Store Abstraction Library written in Go.
package valkeyrie

import (
	"context"
	"sort"
	"sync"

	"github.com/kvtools/valkeyrie/store"
)

var (
	constructorsMu sync.RWMutex
	constructors   = make(map[string]Constructor)
)

// Config the raw type of the store configurations.
type Config any

// Constructor The signature of a store constructor.
type Constructor func(ctx context.Context, endpoints []string, options Config) (store.Store, error)

// Register makes a store constructor available by the provided name.
// If Register is called twice with the same name or if constructor is nil, it panics.
func Register(name string, cttr Constructor) {
	constructorsMu.Lock()
	defer constructorsMu.Unlock()

	if cttr == nil {
		panic("valkeyrie: Register constructor is nil")
	}

	if _, dup := constructors[name]; dup {
		panic("valkeyrie: Register called twice for constructor " + name)
	}

	constructors[name] = cttr
}

// Unregister Unregisters a store.
func Unregister(storeName string) {
	constructorsMu.Lock()
	defer constructorsMu.Unlock()

	delete(constructors, storeName)
}

// UnregisterAllConstructors Unregisters all stores.
func UnregisterAllConstructors() {
	constructorsMu.Lock()
	defer constructorsMu.Unlock()

	constructors = make(map[string]Constructor)
}

// Constructors returns a sorted list of the names of the registered constructors.
func Constructors() []string {
	constructorsMu.RLock()
	defer constructorsMu.RUnlock()

	list := make([]string, 0, len(constructors))
	for name := range constructors {
		list = append(list, name)
	}

	sort.Strings(list)

	return list
}

// NewStore creates a new store instance.
func NewStore(ctx context.Context, storeName string, endpoints []string, options Config) (store.Store, error) {
	constructorsMu.RLock()
	construct, ok := constructors[storeName]
	constructorsMu.RUnlock()

	if !ok {
		return nil, &store.UnknownConstructorError{Store: storeName}
	}

	if construct == nil {
		return nil, &store.UnknownConstructorError{Store: storeName}
	}

	return construct(ctx, endpoints, options)
}
