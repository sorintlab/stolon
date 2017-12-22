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
	"context"
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

func TestInit(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tstore := setupStore(t, dir)
	defer tstore.Stop()

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	clusterName := uuid.NewV4().String()

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		PGParameters:       defaultPGParameters,
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
	defer ts.Stop()
	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk.Stop()

	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("database is up")
}

func TestInitNewMerge(t *testing.T) {
	t.Parallel()
	testInitNew(t, true)
}

func TestInitNewNoMerge(t *testing.T) {
	t.Parallel()
	testInitNew(t, false)
}

func testInitNew(t *testing.T, merge bool) {
	clusterName := uuid.NewV4().String()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tstore := setupStore(t, dir)
	defer tstore.Stop()

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StoreBasePath, clusterName)

	sm := store.NewStore(tstore.store, storePath)

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		FailInterval:       &cluster.Duration{Duration: 10 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		MergePgParameters:  &merge,
		PGParameters:       defaultPGParameters,
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

	cd, _, err := sm.GetClusterData(context.TODO())
	// max_connection should be set by initdb
	_, ok := cd.Cluster.Spec.PGParameters["max_connections"]
	if merge && !ok {
		t.Fatalf("expected max_connection set in cluster data pgParameters")
	}
	if !merge && ok {
		t.Fatalf("expected no max_connection set in cluster data pgParameters")
	}

	tk.Stop()
}

func TestInitExistingMerge(t *testing.T) {
	t.Parallel()
	testInitExisting(t, true)
}

func TestInitExistingNoMerge(t *testing.T) {
	t.Parallel()
	testInitExisting(t, false)
}

func testInitExisting(t *testing.T, merge bool) {
	clusterName := uuid.NewV4().String()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tstore := setupStore(t, dir)
	defer tstore.Stop()

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StoreBasePath, clusterName)

	sm := store.NewStore(tstore.store, storePath)

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		PGParameters: pgParametersWithDefaults(cluster.PGParameters{
			"archive_mode": "on",
		}),
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

	if err := populate(t, tk); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, tk, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Now initialize a new cluster with the existing keeper
	initialClusterSpec = &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeExisting),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		MergePgParameters:  &merge,
		PGParameters:       defaultPGParameters,
		ExistingConfig: &cluster.ExistingConfig{
			KeeperUID: tk.uid,
		},
	}
	initialClusterSpecFile, err = writeClusterSpec(dir, initialClusterSpec)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	t.Logf("reinitializing cluster")
	// Initialize cluster with new spec
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "init", "-y", "-f", initialClusterSpecFile)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := WaitClusterPhase(sm, cluster.ClusterPhaseInitializing, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := WaitClusterPhase(sm, cluster.ClusterPhaseNormal, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, tk)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	pgParameters, err := tk.GetPGParameters()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	v, ok := pgParameters["archive_mode"]
	if merge && v != "on" {
		t.Fatalf("expected archive_mode == on got %q", v)
	}
	if !merge && ok {
		t.Fatalf("expected archive_mode empty")
	}

	cd, _, err := sm.GetClusterData(context.TODO())
	// max_connection should be set by initdb
	v, ok = cd.Cluster.Spec.PGParameters["archive_mode"]
	if merge && v != "on" {
		t.Fatalf("expected archive_mode == on got %q", v)
	}
	if !merge && ok {
		t.Fatalf("expected archive_mode empty")
	}

	tk.Stop()
}

func TestInitUsers(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tstore := setupStore(t, dir)
	defer tstore.Stop()

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	// Test pg-repl-username == pg-su-username but password different
	clusterName := uuid.NewV4().String()
	tk, err := NewTestKeeper(t, dir, clusterName, "user01", "password01", "user01", "password02", tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.StartExpect(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk.Stop()
	if err := tk.cmd.Expect("provided superuser name and replication user name are the same but provided passwords are different"); err != nil {
		t.Fatalf("expecting keeper reporting provided superuser name and replication user name are the same but provided passwords are different")
	}

	// Test pg-repl-username == pg-su-username
	clusterName = uuid.NewV4().String()
	storePath := filepath.Join(common.StoreBasePath, clusterName)

	sm := store.NewStore(tstore.store, storePath)

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		PGParameters:       defaultPGParameters,
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
	defer ts.Stop()

	if err := WaitClusterPhase(sm, cluster.ClusterPhaseInitializing, 30*time.Second); err != nil {
		t.Fatal("expected cluster in initializing phase")
	}

	tk2, err := NewTestKeeper(t, dir, clusterName, "user01", "password", "user01", "password", tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk2.StartExpect(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk2.Stop()
	if err := tk2.cmd.ExpectTimeout("replication role added to superuser", 60*time.Second); err != nil {
		t.Fatalf("expecting keeper reporting replication role added to superuser")
	}

	// Test pg-repl-username != pg-su-username and pg-su-password defined
	clusterName = uuid.NewV4().String()
	storePath = filepath.Join(common.StoreBasePath, clusterName)

	sm = store.NewStore(tstore.store, storePath)

	ts2, err := NewTestSentinel(t, dir, clusterName, tstore.storeBackend, storeEndpoints, fmt.Sprintf("--initial-cluster-spec=%s", initialClusterSpecFile))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := ts2.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer ts2.Stop()

	if err := WaitClusterPhase(sm, cluster.ClusterPhaseInitializing, 60*time.Second); err != nil {
		t.Fatal("expected cluster in initializing phase")
	}

	tk3, err := NewTestKeeper(t, dir, clusterName, "user01", "password", "user02", "password", tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk3.StartExpect(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk3.Stop()
	if err := tk3.cmd.ExpectTimeout("superuser password set", 60*time.Second); err != nil {
		t.Fatalf("expecting keeper reporting superuser password set")
	}
	if _, err := tk3.cmd.ExpectTimeoutRegexFind(`replication role created\s+{"role": "user02"}`, 60*time.Second); err != nil {
		t.Fatalf("expecting keeper reporting replication role user02 created")
	}
}

func TestInitialClusterSpec(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tstore := setupStore(t, dir)
	defer tstore.Stop()

	clusterName := uuid.NewV4().String()

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StoreBasePath, clusterName)

	sm := store.NewStore(tstore.store, storePath)

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:          &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:           &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout:     &cluster.Duration{Duration: 30 * time.Second},
		SynchronousReplication: cluster.BoolP(true),
		PGParameters:           defaultPGParameters,
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
	defer ts.Stop()

	if err := WaitClusterPhase(sm, cluster.ClusterPhaseInitializing, 60*time.Second); err != nil {
		t.Fatal("expected cluster in initializing phase")
	}

	cd, _, err := sm.GetClusterData(context.TODO())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !*cd.Cluster.Spec.SynchronousReplication {
		t.Fatal("expected cluster spec with SynchronousReplication enabled")
	}
}

func TestExclusiveLock(t *testing.T) {
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

	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	tk1, err := NewTestKeeperWithID(t, dir, id, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk1.StartExpect(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk1.Stop()

	// Wait for tk1 to take exclusive lock
	if err := tk1.cmd.ExpectTimeout("exclusive lock on data dir taken", 60*time.Second); err != nil {
		t.Fatalf("expecting keeper reporting that exclusive lock on data dir has been taken")
	}

	tk2, err := NewTestKeeperWithID(t, dir, id, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk2.StartExpect(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk2.Stop()

	// Wait for tk1 to take exclusive lock
	if err := tk2.cmd.ExpectTimeout("cannot take exclusive lock on data dir", 60*time.Second); err != nil {
		t.Fatalf("expecting keeper reporting that failed to take an exclusive lock on data dir")
	}
}
