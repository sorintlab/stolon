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

func TestServerParameters(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tstore, err := NewTestStore(t, dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tstore.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tstore.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store up: %v", err)
	}
	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	defer tstore.Stop()

	clusterName := uuid.NewV4().String()

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
		}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.StartExpect(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk.Stop()

	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	pair, err = e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 2,
			Master:  tk.id,
			KeepersRole: cluster.KeepersRole{
				tk.id: &cluster.KeeperRole{ID: tk.id, Follow: ""},
			},
			Config: &cluster.NilConfig{
				SleepInterval:      &cluster.Duration{5 * time.Second},
				KeeperFailInterval: &cluster.Duration{10 * time.Second},
				PGParameters: &map[string]string{
					"unexistent_parameter": "value",
				},
			},
		}, pair)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tk.cmd.ExpectTimeout("postgres parameters changed, reloading postgres instance", 30*time.Second)

	// On the next keeper check they shouldn't be changed
	tk.cmd.ExpectTimeout("postgres parameters not changed", 30*time.Second)

	tk.Stop()

	// Start tk again, postgres should fail to start due to bad parameter
	if err := tk.StartExpect(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tk.cmd.ExpectTimeout("failed to start postgres:", 30*time.Second)

	// Fix wrong parameters
	pair, err = e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 2,
			Master:  tk.id,
			KeepersRole: cluster.KeepersRole{
				tk.id: &cluster.KeeperRole{ID: tk.id, Follow: ""},
			},
			Config: &cluster.NilConfig{
				SleepInterval:      &cluster.Duration{5 * time.Second},
				KeeperFailInterval: &cluster.Duration{10 * time.Second},
				PGParameters:       &map[string]string{},
			},
		}, pair)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.WaitDBUp(30 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
