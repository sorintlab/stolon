# Valkeyrie Consul

[![GoDoc](https://godoc.org/github.com/kvtools/consul?status.png)](https://godoc.org/github.com/kvtools/consul)
[![Build Status](https://github.com/kvtools/consul/actions/workflows/build.yml/badge.svg)](https://github.com/kvtools/consul/actions/workflows/build.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kvtools/consul)](https://goreportcard.com/report/github.com/kvtools/consul)

[`valkeyrie`](https://github.com/kvtools/valkeyrie) provides a Go native library to store metadata using Distributed Key/Value stores (or common databases).

## Compatibility

A **storage backend** in `valkeyrie` implements (fully or partially) the [Store](https://github.com/kvtools/valkeyrie/blob/master/store/store.go#L69) interface.

| Calls                 | Consul |
|-----------------------|:------:|
| Put                   |  🟢️   |
| Get                   |  🟢️   |
| Delete                |  🟢️   |
| Exists                |  🟢️   |
| Watch                 |  🟢️   |
| WatchTree             |  🟢️   |
| NewLock (Lock/Unlock) |  🟢️   |
| List                  |  🟢️   |
| DeleteTree            |  🟢️   |
| AtomicPut             |  🟢️   |
| AtomicDelete          |  🟢️   |

## Supported Versions

Consul versions >= `0.5.1` because it uses Sessions with `Delete` behavior for the use of `TTLs` (mimics zookeeper's Ephemeral node support).
If you don't plan to use `TTLs`: you can use Consul version `0.4.0+`.

## Examples

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/kvtools/consul"
	"github.com/kvtools/valkeyrie"
)

func main() {
	ctx := context.Background()

	config := &consul.Config{
		ConnectionTimeout: 10 * time.Second,
	}

	kv, err := valkeyrie.NewStore(ctx, consul.StoreName, []string{"localhost:8500"}, config)
	if err != nil {
		log.Fatal("Cannot create store")
	}

	key := "foo"

	err = kv.Put(ctx, key, []byte("bar"), nil)
	if err != nil {
		log.Fatalf("Error trying to put value at key: %v", key)
	}

	pair, err := kv.Get(ctx, key, nil)
	if err != nil {
		log.Fatalf("Error trying accessing value at key: %v", key)
	}

	log.Printf("value: %s", string(pair.Value))
	
	err = kv.Delete(ctx, key)
	if err != nil {
		log.Fatalf("Error trying to delete key %v", key)
	}
}
```
