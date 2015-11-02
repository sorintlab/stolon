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
	etcdm "github.com/sorintlab/stolon/pkg/etcd"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
)

func TestServerParameters(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	te, err := NewTestEtcd(dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := te.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := te.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on etcd up: %v", err)
	}
	etcdEndpoints := fmt.Sprintf("http://%s:%s", te.listenAddress, te.port)
	defer te.Stop()

	clusterName := uuid.NewV4().String()

	etcdPath := filepath.Join(common.EtcdBasePath, clusterName)
	e, err := etcdm.NewEtcdManager(etcdEndpoints, etcdPath, common.DefaultEtcdRequestTimeout)
	if err != nil {
		t.Fatalf("cannot create etcd manager: %v", err)
	}

	res, err := e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:      &cluster.Duration{5 * time.Second},
				KeeperFailInterval: &cluster.Duration{10 * time.Second},
			},
		}, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	prevCDIndex := res.Node.ModifiedIndex

	tk, err := NewTestKeeper(dir, clusterName, etcdEndpoints)
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

	res, err = e.SetClusterData(cluster.KeepersState{},
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
		}, prevCDIndex)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	prevCDIndex = res.Node.ModifiedIndex

	tk.cmd.Expect("postgres parameters changed, reloading postgres instance")

	// On the next keeper check they shouldn't be changed
	tk.cmd.Expect("postgres parameters not changed")

	tk.Stop()

	// Start tk again, postgres should fail to start due to bad parameter
	if err := tk.StartExpect(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tk.cmd.Expect("failed to start postgres:")

	// Fix wrong parameters
	res, err = e.SetClusterData(cluster.KeepersState{},
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
		}, prevCDIndex)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	prevCDIndex = res.Node.ModifiedIndex

	if err := tk.WaitDBUp(30 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
