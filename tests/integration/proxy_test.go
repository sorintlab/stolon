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

package integration

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/store"

	"github.com/satori/go.uuid"
)

func TestProxyListening(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tstore, err := NewTestStore(t, dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	tp, err := NewTestProxy(t, dir, clusterName, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tp.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tp.Stop()

	t.Logf("test proxy start with store down. Should not listen")
	// tp should not listen because it cannot talk with store
	if err := tp.WaitNotListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp not listening due to failed store communication, but it's listening.")
	}

	tp.Stop()

	if err := tstore.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tstore.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store up: %v", err)
	}
	defer func() {
		if tstore.cmd != nil {
			tstore.Stop()
		}
	}()

	storePath := filepath.Join(common.StoreBasePath, clusterName)

	kvstore, err := store.NewStore(tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("cannot create store: %v", err)
	}

	e := store.NewStoreManager(kvstore, storePath)

	pair, err := e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:      &cluster.Duration{5 * time.Second},
				KeeperFailInterval: &cluster.Duration{10 * time.Second},
			},
			ProxyConf: &cluster.ProxyConf{
				// fake pg address, not relevant
				Host: "localhost",
				Port: "5432",
			},
		}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// test proxy start with the store up
	t.Logf("test proxy start with the store up. Should listen")
	if err := tp.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test proxy error communicating with store. Should stop listening")
	// Stop store
	tstore.Stop()
	if err := tstore.WaitDown(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store down: %v", err)
	}

	// tp should not listen because it cannot talk with the store
	if err := tp.WaitNotListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp not listening due to failed store communication, but it's listening.")
	}

	t.Logf("test proxy communication with store restored. Should start listening")
	// Start store
	if err := tstore.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tstore.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store up: %v", err)
	}
	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test proxyConf removed. Should continue listening")
	// remove proxyConf
	pair, err = e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:      &cluster.Duration{5 * time.Second},
				KeeperFailInterval: &cluster.Duration{10 * time.Second},
			},
			ProxyConf: nil,
		}, pair)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test proxyConf restored. Should continue listening")
	// Set proxyConf again
	pair, err = e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:      &cluster.Duration{5 * time.Second},
				KeeperFailInterval: &cluster.Duration{10 * time.Second},
			},
			ProxyConf: &cluster.ProxyConf{
				// fake pg address, not relevant
				Host: "localhost",
				Port: "5432",
			},
		}, pair)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test clusterView removed. Should continue listening")
	// remove whole clusterview
	_, err = e.SetClusterData(cluster.KeepersState{}, nil, pair)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}
}
