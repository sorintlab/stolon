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

	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/common"
	"github.com/sorintlab/stolon/internal/store"

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

	storePath := filepath.Join(common.StorePrefix, clusterName)

	sm := store.NewKVBackedStore(tstore.store, storePath)

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

func TestWalLevel(t *testing.T) {
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

	storePath := filepath.Join(common.StorePrefix, clusterName)

	sm := store.NewKVBackedStore(tstore.store, storePath)

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

	// "archive" isn't an accepted wal_level
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "pgParameters" : { "wal_level": "archive" } }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.cmd.ExpectTimeout("postgres parameters not changed", 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tk.Stop()
	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	pgParameters, err := tk.GetPGParameters()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	walLevel := pgParameters["wal_level"]
	if walLevel != "replica" && walLevel != "hot_standby" {
		t.Fatalf("unexpected wal_level value: %q", walLevel)
	}

	// "logical" is an accepted wal_level
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "pgParameters" : { "wal_level": "logical" } }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.cmd.ExpectTimeout("postgres parameters changed, reloading postgres instance", 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tk.Stop()
	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	pgParameters, err = tk.GetPGParameters()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	walLevel = pgParameters["wal_level"]
	if walLevel != "logical" {
		t.Fatalf("unexpected wal_level value: %q", walLevel)
	}
}

func TestAlterSystem(t *testing.T) {
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

	storePath := filepath.Join(common.StorePrefix, clusterName)

	sm := store.NewKVBackedStore(tstore.store, storePath)

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

	expectedErr := `pq: could not fsync file "postgresql.auto.conf": Invalid argument`
	if _, err := tk.Exec("alter system set archive_mode to on"); err != nil {
		if err.Error() != expectedErr {
			t.Fatalf("expected err: %q, got: %q", expectedErr, err)
		}
	} else {
		t.Fatalf("expected err: %q, got no error", expectedErr)
	}

	tk.Stop()
	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	pgParameters, err := tk.GetPGParameters()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v, ok := pgParameters["archive_mode"]; ok {
		t.Fatalf("expected archive_mode not defined, got value: %q", v)
	}
}

func TestAdditionalReplicationSlots(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tp, tstore := setupServers(t, clusterName, dir, 2, 1, false, false, nil)
	defer shutdown(tks, tss, tp, tstore)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StorePrefix, clusterName)
	sm := store.NewKVBackedStore(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)
	standby := standbys[0]

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err := getLines(t, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}
	if err := waitLines(t, standby, 1, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// create additional replslots on master
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "additionalMasterReplicationSlots" : [ "replslot01", "replslot02" ] }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := waitNotStolonReplicationSlots(master, []string{"replslot01", "replslot02"}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// no repl slot on standby
	if err := waitNotStolonReplicationSlots(standby, []string{}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// remove replslot02
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "additionalMasterReplicationSlots" : [ "replslot01" ] }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := waitNotStolonReplicationSlots(master, []string{"replslot01"}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// no repl slot on standby
	if err := waitNotStolonReplicationSlots(standby, []string{}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// remove additional replslots on master
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "additionalMasterReplicationSlots" : null }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := waitNotStolonReplicationSlots(master, []string{}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// no repl slot on standby
	if err := waitNotStolonReplicationSlots(standby, []string{}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// create additional replslots on master
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "additionalMasterReplicationSlots" : [ "replslot01", "replslot02" ] }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := waitNotStolonReplicationSlots(master, []string{"replslot01", "replslot02"}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// no repl slot on standby
	if err := waitNotStolonReplicationSlots(standby, []string{}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Manually create a replication slot. It should be dropped.
	if _, err := master.Exec("select pg_create_physical_replication_slot('manualreplslot')"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := waitNotStolonReplicationSlots(master, []string{"replslot01", "replslot02"}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop the keeper process on master, should also stop the database
	t.Logf("Stopping current master keeper: %s", master.uid)
	master.Stop()

	// Wait for cluster data containing standby as master
	if err := WaitClusterDataMaster(standby.uid, sm, 30*time.Second); err != nil {
		t.Fatalf("expected master %q in cluster view", standby.uid)
	}
	if err := standby.WaitDBRole(common.RoleMaster, nil, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// repl slot on standby which is the new master
	if err := waitNotStolonReplicationSlots(standby, []string{"replslot01", "replslot02"}, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
