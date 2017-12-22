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
	t.Parallel()

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

	sm := store.NewStore(tstore.store, storePath)

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
	}
	initialClusterSpecFile, err := writeClusterSpec(dir, initialClusterSpec)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	ts, err := NewTestSentinel(t, dir, clusterName, tstore.storeBackend, storeEndpoints, fmt.Sprintf("--initial-cluster-spec=%s", initialClusterSpecFile))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := ts.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := WaitClusterPhase(sm, cluster.ClusterPhaseNormal, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "pgParameters" : { "unexistent_parameter": "value" } }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.cmd.ExpectTimeout("postgres parameters changed, reloading postgres instance", 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// On the next keeper check they shouldn't be changed
	if err := tk.cmd.ExpectTimeout("postgres parameters not changed", 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tk.Stop()

	// Start tk again, postgres should fail to start due to bad parameter
	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk.Stop()

	if err := tk.cmd.ExpectTimeout("failed to start postgres", 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Fix wrong parameters
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "pgParameters" : null }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.WaitDBUp(30 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
