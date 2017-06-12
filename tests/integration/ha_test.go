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
	"syscall"
	"testing"
	"time"

	"github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/store"
)

const (
	pgReplUsername = "stolon_repluser"
	pgReplPassword = "stolon_replpassword"
	pgSUUsername   = "stolon_superuser"
	pgSUPassword   = "stolon_superuserpassword"
)

type testKeepers map[string]*TestKeeper
type testSentinels map[string]*TestSentinel

func setupStore(t *testing.T, dir string) *TestStore {
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
	return tstore
}

func TestInitWithMultipleKeepers(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tstore := setupStore(t, dir)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	clusterName := uuid.NewV4().String()

	storePath := filepath.Join(common.StoreBasePath, clusterName)

	sm := store.NewStoreManager(tstore.store, storePath)

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		FailInterval:       &cluster.Duration{Duration: 10 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
	}
	initialClusterSpecFile, err := writeClusterSpec(dir, initialClusterSpec)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tks := testKeepers{}
	tss := testSentinels{}

	// Start 3 keepers
	for i := uint8(0); i < 3; i++ {
		tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tk.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tks[tk.uid] = tk
	}

	// Start 2 sentinels
	for i := uint8(0); i < 2; i++ {
		ts, err := NewTestSentinel(t, dir, clusterName, tstore.storeBackend, storeEndpoints, fmt.Sprintf("--initial-cluster-spec=%s", initialClusterSpecFile))
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := ts.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tss[ts.uid] = ts
	}

	defer shutdown(tks, tss, tstore)

	// Wait for clusterView containing a master
	masterUID, err := WaitClusterDataWithMaster(sm, 30*time.Second)
	if err != nil {
		t.Fatal("expected a master in cluster view")
	}
	waitKeeperReady(t, sm, tks[masterUID])
}

func setupServers(t *testing.T, clusterName, dir string, numKeepers, numSentinels uint8, syncRepl bool, usePgrewind bool, primaryKeeper *TestKeeper) (testKeepers, testSentinels, *TestStore) {
	var initialClusterSpec *cluster.ClusterSpec
	if primaryKeeper == nil {
		initialClusterSpec = &cluster.ClusterSpec{
			InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
			SleepInterval:          &cluster.Duration{Duration: 2 * time.Second},
			FailInterval:           &cluster.Duration{Duration: 5 * time.Second},
			ConvergenceTimeout:     &cluster.Duration{Duration: 30 * time.Second},
			MaxStandbyLag:          cluster.Uint32P(50 * 1024), // limit lag to 50kiB
			SynchronousReplication: cluster.BoolP(syncRepl),
			UsePgrewind:            cluster.BoolP(usePgrewind),
			PGParameters:           defaultPGParameters,
		}
	} else {
		// if primaryKeeper is provided then we should create a standby cluster and do a
		// pitr recovery from the external primary database

		pgpass, err := ioutil.TempFile(dir, "pgpass")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		pgpass.WriteString(fmt.Sprintf("%s:%s:*:%s:%s\n", primaryKeeper.pgListenAddress, primaryKeeper.pgPort, primaryKeeper.pgReplUsername, primaryKeeper.pgReplPassword))
		pgpass.Close()

		initialClusterSpec = &cluster.ClusterSpec{
			InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModePITR),
			Role:                   cluster.ClusterRoleP(cluster.ClusterRoleStandby),
			SleepInterval:          &cluster.Duration{Duration: 2 * time.Second},
			FailInterval:           &cluster.Duration{Duration: 5 * time.Second},
			ConvergenceTimeout:     &cluster.Duration{Duration: 30 * time.Second},
			MaxStandbyLag:          cluster.Uint32P(50 * 1024), // limit lag to 50kiB
			SynchronousReplication: cluster.BoolP(syncRepl),
			PGParameters:           defaultPGParameters,
			PITRConfig: &cluster.PITRConfig{
				DataRestoreCommand: fmt.Sprintf("PGPASSFILE=%s pg_basebackup -D %%d -h %s -p %s -U %s", pgpass.Name(), primaryKeeper.pgListenAddress, primaryKeeper.pgPort, primaryKeeper.pgReplUsername),
			},
			StandbySettings: &cluster.StandbySettings{
				PrimaryConninfo: fmt.Sprintf("sslmode=disable host=%s port=%s user=%s password=%s", primaryKeeper.pgListenAddress, primaryKeeper.pgPort, primaryKeeper.pgReplUsername, primaryKeeper.pgReplPassword),
			},
		}
	}

	return setupServersCustom(t, clusterName, dir, numKeepers, numSentinels, initialClusterSpec)
}

func setupServersCustom(t *testing.T, clusterName, dir string, numKeepers, numSentinels uint8, initialClusterSpec *cluster.ClusterSpec) (testKeepers, testSentinels, *TestStore) {
	tstore := setupStore(t, dir)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	initialClusterSpecFile, err := writeClusterSpec(dir, initialClusterSpec)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tks := map[string]*TestKeeper{}
	tss := map[string]*TestSentinel{}

	// Start sentinels
	for i := uint8(0); i < numSentinels; i++ {
		ts, err := NewTestSentinel(t, dir, clusterName, tstore.storeBackend, storeEndpoints, fmt.Sprintf("--initial-cluster-spec=%s", initialClusterSpecFile))
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := ts.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tss[ts.uid] = ts
	}

	// Start other keepers
	for i := uint8(0); i < numKeepers; i++ {
		tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tk.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tks[tk.uid] = tk
	}
	return tks, tss, tstore
}

func populate(t *testing.T, tk *TestKeeper) error {
	_, err := tk.Exec("CREATE TABLE table01(ID INT PRIMARY KEY NOT NULL, VALUE INT NOT NULL)")
	return err
}

func write(t *testing.T, tk *TestKeeper, id, value int) error {
	_, err := tk.Exec("INSERT INTO table01 VALUES ($1, $2)", id, value)
	return err
}

func getLines(t *testing.T, tk *TestKeeper) (int, error) {
	rows, err := tk.Query("SELECT FROM table01")
	if err != nil {
		return 0, err
	}
	c := 0
	for rows.Next() {
		c++
	}
	return c, rows.Err()
}

func waitLines(t *testing.T, tk *TestKeeper, num int, timeout time.Duration) error {
	start := time.Now()
	c := -1
	for time.Now().Add(-timeout).Before(start) {
		c, err := getLines(t, tk)
		if err != nil {
			goto end
		}
		if c == num {
			return nil
		}
	end:
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %d lines, got: %d", num, c)
}

func shutdown(tks map[string]*TestKeeper, tss map[string]*TestSentinel, tstore *TestStore) {
	for _, ts := range tss {
		if ts.cmd != nil {
			ts.Stop()
		}
	}
	for _, tk := range tks {
		if tk.cmd != nil {
			tk.Stop()
		}
	}
	if tstore.cmd != nil {
		tstore.Kill()
	}
}

func waitKeeperReady(t *testing.T, sm *store.StoreManager, keeper *TestKeeper) {
	if err := WaitClusterDataKeeperInitialized(keeper.uid, sm, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := keeper.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func waitMasterStandbysReady(t *testing.T, sm *store.StoreManager, tks testKeepers) (master *TestKeeper, standbys []*TestKeeper) {
	// Wait for normal cluster phase (master ready)
	masterUID, err := WaitClusterDataWithMaster(sm, 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	master = tks[masterUID]
	// Return all other keepers as standbys (assume that MaxStandbysPerSender is greater the the number of keepers)
	for _, tk := range tks {
		if tk.uid == masterUID {
			continue
		}
		standbys = append(standbys, tk)
	}
	waitKeeperReady(t, sm, master)
	for _, standby := range standbys {
		waitKeeperReady(t, sm, standby)
	}
	return
}

func testMasterStandby(t *testing.T, syncRepl bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, false, nil)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)
	standby := standbys[0]

	if syncRepl {
		if err := WaitClusterDataSynchronousStandbys([]string{standby.uid}, sm, 30*time.Second); err != nil {
			t.Fatalf("expected synchronous standby on keeper %q in cluster data", standby.uid)
		}
	}

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
	if err := waitLines(t, standby, 1, 10*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestMasterStandby(t *testing.T) {
	t.Parallel()
	testMasterStandby(t, false)
}

func TestMasterStandbySyncRepl(t *testing.T) {
	t.Parallel()
	testMasterStandby(t, true)
}

func testFailover(t *testing.T, syncRepl bool, standbyCluster bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	var ptk *TestKeeper
	var primary *TestKeeper
	if standbyCluster {
		primaryClusterName := uuid.NewV4().String()
		ptks, ptss, ptstore := setupServers(t, primaryClusterName, dir, 1, 1, false, false, nil)
		defer shutdown(ptks, ptss, ptstore)
		for _, ptk = range ptks {
			break
		}
		primary = ptk
	}

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, false, ptk)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)
	standby := standbys[0]

	if !standbyCluster {
		primary = master
	}

	// a standby cluster will disable syncRepl since it's not possible to do sync repl on cascading standbys
	if syncRepl && !standbyCluster {
		if err := WaitClusterDataSynchronousStandbys([]string{standby.uid}, sm, 30*time.Second); err != nil {
			t.Fatalf("expected synchronous standby on keeper %q in cluster data", standby.uid)
		}
	}

	if err := populate(t, primary); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, primary, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// wait for the keepers to have reported their state (needed to know the instance XLogPos)
	time.Sleep(5 * time.Second)
	WaitClusterSyncedXLogPos([]string{master.uid, standby.uid}, sm, 20*time.Second)

	// Stop the keeper process on master, should also stop the database
	t.Logf("Stopping current master keeper: %s", master.uid)
	master.Stop()

	// Wait for cluster data containing standby as master
	if err := WaitClusterDataMaster(standby.uid, sm, 30*time.Second); err != nil {
		t.Fatalf("expected master %q in cluster view", standby.uid)
	}
	if err := standby.WaitDBRole(common.RoleMaster, ptk, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !standbyCluster {
		primary = standby
	}

	c, err := getLines(t, standby)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}
}

func TestFailover(t *testing.T) {
	t.Parallel()
	testFailover(t, false, false)
}

func TestFailoverSyncRepl(t *testing.T) {
	t.Parallel()
	testFailover(t, true, false)
}

func TestFailoverStandbyCluster(t *testing.T) {
	t.Parallel()
	testFailover(t, false, true)
}

func TestFailoverSyncReplStandbyCluster(t *testing.T) {
	t.Parallel()
	testFailover(t, false, true)
}

// Tests standby elected as new master but fails to become master. Then old
// master comes back and is re-elected as master.
func testFailoverFailed(t *testing.T, syncRepl bool, standbyCluster bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	var ptk *TestKeeper
	var primary *TestKeeper
	if standbyCluster {
		primaryClusterName := uuid.NewV4().String()
		ptks, ptss, ptstore := setupServers(t, primaryClusterName, dir, 1, 1, false, false, nil)
		defer shutdown(ptks, ptss, ptstore)
		for _, ptk = range ptks {
			break
		}
		primary = ptk
	}

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, false, ptk)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)
	standby := standbys[0]

	if !standbyCluster {
		primary = master
	}

	// a standby cluster will disable syncRepl since it's not possible to do sync repl on cascading standbys
	if syncRepl && !standbyCluster {
		if err := WaitClusterDataSynchronousStandbys([]string{standby.uid}, sm, 30*time.Second); err != nil {
			t.Fatalf("expected synchronous standby on keeper %q in cluster data", standby.uid)
		}
	}

	if err := populate(t, primary); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, primary, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// wait for the keepers to have reported their state (needed to know the instance XLogPos)
	time.Sleep(5 * time.Second)
	WaitClusterSyncedXLogPos([]string{master.uid, standby.uid}, sm, 20*time.Second)

	// Stop the keeper process on master, should also stop the database
	t.Logf("Stopping current master keeper: %s", master.uid)
	master.Stop()

	// Wait for cluster data containing standby as master
	if err := WaitClusterDataMaster(standby.uid, sm, 30*time.Second); err != nil {
		t.Fatalf("expected master %q in cluster view", standby.uid)
	}

	// Stopping standby before reading the new cluster data and promoting
	// TODO(sgotti) this is flacky and the standby can read the data and
	// publish new state before it's stopped
	t.Logf("Stopping current standby keeper: %s", standby.uid)
	standby.Stop()

	t.Logf("Starting previous master keeper: %s", master.uid)
	master.Start()
	// Wait for cluster data containing previous master as master
	err = WaitClusterDataMaster(master.uid, sm, 30*time.Second)
	if !syncRepl && err != nil {
		t.Fatalf("expected master %q in cluster view", master.uid)
	}
	if syncRepl && !standbyCluster {
		if err == nil {
			t.Fatalf("expected timeout since with synchronous replication the old master shouldn't be elected as master")
		}
	}
}

func TestFailoverFailed(t *testing.T) {
	t.Parallel()
	testFailoverFailed(t, false, false)
}

func TestFailoverFailedSyncRepl(t *testing.T) {
	t.Parallel()
	testFailoverFailed(t, true, false)
}

func TestFailoverFailedStandbyCluster(t *testing.T) {
	t.Parallel()
	testFailoverFailed(t, true, true)
}

// test that a standby with a lag (reported) greater than MaxStandbyLag from the
// master (reported) xlogpos won't be elected as the new master. This test is
// valid only for asynchronous replication
func testFailoverTooMuchLag(t *testing.T, standbyCluster bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	var ptk *TestKeeper
	var primary *TestKeeper
	if standbyCluster {
		primaryClusterName := uuid.NewV4().String()
		ptks, ptss, ptstore := setupServers(t, primaryClusterName, dir, 1, 1, false, false, nil)
		defer shutdown(ptks, ptss, ptstore)
		for _, ptk = range ptks {
			break
		}
		primary = ptk
	}

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, false, false, ptk)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)
	standby := standbys[0]

	if !standbyCluster {
		primary = master
	}

	if err := populate(t, primary); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// stop the standby and write more than MaxStandbyLag data to the master
	t.Logf("Stopping current standby keeper: %s", standby.uid)
	standby.Stop()
	if err := standby.WaitDBDown(30 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	for i := 1; i < 1000; i++ {
		if err := write(t, primary, i, i); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	}

	// wait for the master to have reported its state
	time.Sleep(5 * time.Second)

	// Stop the keeper process on master, should also stop the database
	t.Logf("Stopping current master keeper: %s", master.uid)
	master.Stop()
	// start the standby
	t.Logf("Starting current standby keeper: %s", standby.uid)
	standby.Start()

	// standby shouldn't be elected as master since its lag is greater than MaxStandbyLag
	if err := standby.WaitDBRole(common.RoleMaster, ptk, 30*time.Second); err == nil {
		t.Fatalf("standby shouldn't be elected as master")
	}
}

func TestFailoverTooMuchLag(t *testing.T) {
	t.Parallel()
	testFailoverTooMuchLag(t, false)
}

func TestFailoverTooMuchLagStandbyCluster(t *testing.T) {
	t.Parallel()
	testFailoverTooMuchLag(t, true)
}

func testOldMasterRestart(t *testing.T, syncRepl, usePgrewind bool, standbyCluster bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	var ptk *TestKeeper
	var primary *TestKeeper
	if standbyCluster {
		primaryClusterName := uuid.NewV4().String()
		ptks, ptss, ptstore := setupServers(t, primaryClusterName, dir, 1, 1, false, false, nil)
		defer shutdown(ptks, ptss, ptstore)
		for _, ptk = range ptks {
			break
		}
		primary = ptk
	}

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, usePgrewind, ptk)
	defer shutdown(tks, tss, tstore)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)

	if !standbyCluster {
		primary = master
	}

	if syncRepl && !standbyCluster {
		if err := WaitClusterDataSynchronousStandbys([]string{standbys[0].uid}, sm, 30*time.Second); err != nil {
			t.Fatalf("expected synchronous standby on keeper %q in cluster data", standbys[0].uid)
		}
	}

	if err := populate(t, primary); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, primary, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// wait for the keepers to have reported their state (needed to know the instance XLogPos)
	time.Sleep(5 * time.Second)
	WaitClusterSyncedXLogPos([]string{master.uid, standbys[0].uid}, sm, 20*time.Second)

	// Stop the keeper process on master, should also stop the database
	t.Logf("Stopping current master keeper: %s", master.uid)
	master.Stop()

	if err := standbys[0].WaitDBRole(common.RoleMaster, ptk, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	// Add another standby so we'll have 2 standbys. With only 1 standby,
	// when using synchronous replication, the test will block forever when
	// writing to the new master since there's not active synchronous
	// standby.
	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tks[tk.uid] = tk

	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	standbys = append(standbys, tk)

	// Wait replicated data to standby
	if err := waitLines(t, standbys[1], 1, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !standbyCluster {
		primary = standbys[0]
	}
	if err := write(t, primary, 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Restart the old master
	t.Logf("Restarting old master keeper: %s", master.uid)
	if err := master.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Wait old master synced with standby
	if err := waitLines(t, master, 2, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := master.WaitDBRole(common.RoleStandby, ptk, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestOldMasterRestart(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, false, false, false)
}

func TestOldMasterRestartSyncRepl(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, true, false, false)
}

func TestOldMasterRestartPgrewind(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, false, true, false)
}

func TestOldMasterRestartSyncReplPgrewind(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, true, true, false)
}

func TestOldMasterRestartStandbyCluster(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, false, false, true)
}

func testPartition1(t *testing.T, syncRepl, usePgrewind bool, standbyCluster bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	var ptk *TestKeeper
	var primary *TestKeeper
	if standbyCluster {
		primaryClusterName := uuid.NewV4().String()
		ptks, ptss, ptstore := setupServers(t, primaryClusterName, dir, 1, 1, false, false, nil)
		defer shutdown(ptks, ptss, ptstore)
		for _, ptk = range ptks {
			break
		}
		primary = ptk
	}

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, usePgrewind, ptk)
	defer shutdown(tks, tss, tstore)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)

	if !standbyCluster {
		primary = master
	}

	if syncRepl && !standbyCluster {
		if err := WaitClusterDataSynchronousStandbys([]string{standbys[0].uid}, sm, 30*time.Second); err != nil {
			t.Fatalf("expected synchronous standby on keeper %q in cluster data", standbys[0].uid)
		}
	}

	if err := populate(t, primary); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, primary, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// wait for the keepers to have reported their state (needed to know the instance XLogPos)
	time.Sleep(5 * time.Second)
	WaitClusterSyncedXLogPos([]string{master.uid, standbys[0].uid}, sm, 20*time.Second)

	// Freeze the keeper and postgres processes on the master
	t.Logf("SIGSTOPping current master keeper: %s", master.uid)
	if err := master.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("SIGSTOPping current master postgres: %s", master.uid)
	if err := master.SignalPG(syscall.SIGSTOP); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[0].WaitDBRole(common.RoleMaster, ptk, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	// Add another standby so we'll have 2 standbys. With only 1 standby,
	// when using synchronous replication, the test will block forever when
	// writing to the new master since there's not active synchronous
	// standby.
	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tks[tk.uid] = tk

	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	standbys = append(standbys, tk)

	// Wait replicated data to standby
	if err := waitLines(t, standbys[1], 1, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !standbyCluster {
		primary = standbys[0]
	}
	if err := write(t, primary, 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Make the master come back
	t.Logf("Resuming old master keeper: %s", master.uid)
	if err := master.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("Resuming old master postgres: %s", master.uid)
	if err := master.SignalPG(syscall.SIGCONT); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Wait replicated data to old master
	if err := waitLines(t, master, 2, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := master.WaitDBRole(common.RoleStandby, ptk, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestPartition1(t *testing.T) {
	t.Parallel()
	testPartition1(t, false, false, false)
}

func TestPartition1SyncRepl(t *testing.T) {
	t.Parallel()
	testPartition1(t, true, false, false)
}

func TestPartition1Pgrewind(t *testing.T) {
	t.Parallel()
	testPartition1(t, false, true, false)
}

func TestPartition1SyncReplPgrewind(t *testing.T) {
	t.Parallel()
	testPartition1(t, true, true, false)
}

func TestPartition1StandbyCluster(t *testing.T) {
	t.Parallel()
	testPartition1(t, false, false, true)
}

func testTimelineFork(t *testing.T, syncRepl, usePgrewind bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, usePgrewind, nil)
	defer shutdown(tks, tss, tstore)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)

	if syncRepl {
		if err := WaitClusterDataSynchronousStandbys([]string{standbys[0].uid}, sm, 30*time.Second); err != nil {
			t.Fatalf("expected synchronous standby on keeper %q in cluster data", standbys[0].uid)
		}
	}

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// wait for the keepers to have reported their state (needed to know the instance XLogPos)
	time.Sleep(5 * time.Second)
	WaitClusterSyncedXLogPos([]string{master.uid, standbys[0].uid}, sm, 20*time.Second)

	// Wait replicated data to standby
	if err := waitLines(t, standbys[0], 1, 10*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Add another standby
	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tks[tk.uid] = tk

	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	standbys = append(standbys, tk)

	// Wait replicated data to standby
	if err := waitLines(t, standbys[1], 1, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop one standby
	t.Logf("Stopping standby[0]: %s", standbys[0].uid)
	standbys[0].Stop()
	if err := standbys[0].WaitDBDown(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Write to master (and replicated to remaining standby)
	if err := write(t, master, 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Wait replicated data to standby[1]
	if err := waitLines(t, standbys[1], 2, 10*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop the master and remaining standby[1]
	t.Logf("Stopping master: %s", master.uid)
	master.Stop()
	t.Logf("Stopping standby[1]: %s", standbys[1].uid)
	standbys[1].Stop()
	if err := master.WaitDBDown(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[1].WaitDBDown(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Start standby[0].
	// If synchronous replication is disabled it will be elected as master but it'll be behind (having only one line).
	// If synchronous replication is enabled it won't be elected as master
	t.Logf("Starting standby[0]: %s", standbys[0].uid)
	standbys[0].Start()
	waitKeeperReady(t, sm, standbys[0])
	err = standbys[0].WaitDBRole(common.RoleMaster, nil, 60*time.Second)
	if !syncRepl && err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if syncRepl {
		if err == nil {
			t.Fatalf("expected timeout since with synchronous replication the standby shouldn't be elected as master")
		}
		// end here
		return
	}

	if err := write(t, standbys[0], 3, 3); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, standbys[0], 4, 4); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err := getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 3 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	// Start the other standby, it should be ahead of current on previous timeline and should full resync himself
	t.Logf("Starting standby[1]: %s", standbys[1].uid)
	standbys[1].Start()
	// Standby[1] will start, then it'll detect it's in another timelinehistory,
	// will stop, full resync and start. We have to avoid detecting it up
	// at the first start. Do this waiting for the number of expected lines.

	// TODO(sgotti) sometimes, when using pg_rewind, the rewinded standby needs wals
	// not available anymore on the source master, until we implement better way to
	// detect missing wals we have to just wait for the start timeout (60s) and
	// then a full resync should be executed.
	if err := waitLines(t, standbys[1], 3, 120*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[1].WaitDBRole(common.RoleStandby, nil, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Check that standby 1 is receiving wals
	if err := write(t, standbys[0], 5, 5); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := waitLines(t, standbys[1], 4, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestTimelineFork(t *testing.T) {
	t.Parallel()
	testTimelineFork(t, false, false)
}

func TestTimelineForkSyncRepl(t *testing.T) {
	t.Parallel()
	testTimelineFork(t, true, false)
}

func TestTimelineForkPgrewind(t *testing.T) {
	t.Parallel()
	testTimelineFork(t, false, true)
}

func TestTimelineForkSyncReplPgrewind(t *testing.T) {
	t.Parallel()
	testTimelineFork(t, true, true)
}

// tests that a master restart with changed address for both keeper and
// postgres (without triggering failover since it restart before being marked
// ad failed) make the slave continue to sync using the new address
func testMasterChangedAddress(t *testing.T, standbyCluster bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	var ptk *TestKeeper
	var primary *TestKeeper
	if standbyCluster {
		primaryClusterName := uuid.NewV4().String()
		ptks, ptss, ptstore := setupServers(t, primaryClusterName, dir, 1, 1, false, false, nil)
		defer shutdown(ptks, ptss, ptstore)
		for _, ptk = range ptks {
			break
		}
		primary = ptk
	}

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, false, false, ptk)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)

	if !standbyCluster {
		primary = master
	}

	if err := populate(t, primary); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, primary, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// wait for the keepers to have reported their state (needed to know the instance XLogPos)
	time.Sleep(5 * time.Second)
	WaitClusterSyncedXLogPos([]string{master.uid, standbys[0].uid}, sm, 20*time.Second)

	// Wait standby synced with master
	if err := waitLines(t, standbys[0], 1, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Restart the keeper process on master with new keeper and postgres
	// addresses (in this case only the port is changed)
	t.Logf("Restarting current master keeper %q with different addresses", master.uid)
	master.Stop()
	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	master, err = NewTestKeeperWithID(t, dir, master.uid, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	tks[master.uid] = master

	if !standbyCluster {
		primary = master
	}

	if err := master.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := master.WaitDBRole(common.RoleMaster, ptk, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := write(t, primary, 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Wait standby synced to master with changed address
	if err := waitLines(t, standbys[0], 2, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestMasterChangedAddress(t *testing.T) {
	t.Parallel()
	testMasterChangedAddress(t, false)
}

func TestMasterChangedAddressStandbyCluster(t *testing.T) {
	t.Parallel()
	testMasterChangedAddress(t, true)
}

func TestFailedStandby(t *testing.T) {
	t.Parallel()
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:             cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:        &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:         &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout:   &cluster.Duration{Duration: 30 * time.Second},
		MaxStandbysPerSender: cluster.Uint16P(1),
		PGParameters:         defaultPGParameters,
	}

	// Create 3 keepers
	tks, tss, tstore := setupServersCustom(t, clusterName, dir, 3, 1, initialClusterSpec)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	// Wait for clusterView containing a master
	masterUID, err := WaitClusterDataWithMaster(sm, 30*time.Second)
	if err != nil {
		t.Fatal("expected a master in cluster view")
	}
	master := tks[masterUID]
	waitKeeperReady(t, sm, master)

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

	if err := WaitNumDBs(sm, 2, 30*time.Second); err != nil {
		t.Fatalf("expected 2 DBs in cluster data: %v", err)
	}

	cd, _, err := sm.GetClusterData()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Get current standby

	var standby *TestKeeper
	for _, db := range cd.DBs {
		if db.UID == cd.Cluster.Status.Master {
			continue
		}
		standby = tks[db.Spec.KeeperUID]
	}
	if err := waitLines(t, standby, 1, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop current standby. The other keeper should be choosed as new standby
	t.Logf("Stopping current standby keeper: %s", standby.uid)
	standby.Stop()

	// Wait for other keeper to have a standby db assigned
	var newStandby *TestKeeper
	for _, tk := range tks {
		if tk.uid != master.uid && tk.uid != standby.uid {
			newStandby = tk
		}
	}

	if err := WaitStandbyKeeper(sm, newStandby.uid, 20*time.Second); err != nil {
		t.Fatalf("expected keeper %s to have a standby db assigned: %v", newStandby.uid, err)
	}

	// Wait for new standby declared as good and remove of old standby
	if err := WaitNumDBs(sm, 2, 30*time.Second); err != nil {
		t.Fatalf("expected 2 DBs in cluster data: %v", err)
	}
}

func TestLoweredMaxStandbysPerSender(t *testing.T) {
	t.Parallel()
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:             cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:        &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:         &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout:   &cluster.Duration{Duration: 30 * time.Second},
		MaxStandbysPerSender: cluster.Uint16P(2),
		PGParameters:         defaultPGParameters,
	}

	// Create 3 keepers
	tks, tss, tstore := setupServersCustom(t, clusterName, dir, 3, 1, initialClusterSpec)
	defer shutdown(tks, tss, tstore)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	// Wait for clusterView containing a master
	masterUID, err := WaitClusterDataWithMaster(sm, 30*time.Second)
	if err != nil {
		t.Fatal("expected a master in cluster view")
	}
	master := tks[masterUID]
	waitKeeperReady(t, sm, master)

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

	if err := WaitNumDBs(sm, 3, 30*time.Second); err != nil {
		t.Fatalf("expected 3 DBs in cluster data: %v", err)
	}

	// Set MaxStandbysPerSender to 1
	err = StolonCtl(clusterName, tstore.storeBackend, storeEndpoints, "update", "--patch", `{ "maxStandbysPerSender" : 1 }`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Wait for only 1 standby
	if err := WaitNumDBs(sm, 2, 30*time.Second); err != nil {
		t.Fatalf("expected 2 DBs in cluster data: %v", err)
	}
}

func TestKeeperRemoval(t *testing.T) {
	t.Parallel()
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		// very low DeadKeeperRemovalInterval to test this behavior
		DeadKeeperRemovalInterval: &cluster.Duration{Duration: 10 * time.Second},
		MaxStandbysPerSender:      cluster.Uint16P(1),
		PGParameters:              defaultPGParameters,
	}

	// Create 2 keepers
	tks, tss, tstore := setupServersCustom(t, clusterName, dir, 2, 1, initialClusterSpec)
	defer shutdown(tks, tss, tstore)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys := waitMasterStandbysReady(t, sm, tks)
	standby1 := standbys[0]

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Add another keeper that won't have a db assigned (since MaxStandbysPerSender == 1)
	standby2, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tks[standby2.uid] = standby2

	if err := standby2.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	standbys = append(standbys, standby2)

	// wait for keeper to be added to the cluster data
	if err := WaitClusterDataKeepers([]string{master.uid, standby1.uid, standby2.uid}, sm, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop the keeper process on standby2
	t.Logf("Stopping standby keeper: %s", standby2.uid)
	standby2.Stop()

	// wait for standby2 keeper to be removed from the cluster data since it's dead a without an assigned db
	if err := WaitClusterDataKeepers([]string{master.uid, standby1.uid}, sm, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop the keeper process on master and standby1
	t.Logf("Stopping master keeper: %s", master.uid)
	master.Stop()
	standby1.Stop()

	// wait for a time greater than DeadKeeperRemovalInterval
	time.Sleep(20 * time.Second)
	// the master keeper shouldn't be removed from the cluster data
	if err := WaitClusterDataKeepers([]string{master.uid, standby1.uid}, sm, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// restart the standby2 and master and stop standby1
	t.Logf("Starting master keeper: %s", master.uid)
	master.Start()
	t.Logf("Starting standby keeper: %s", standby2.uid)
	standby2.Start()

	waitKeeperReady(t, sm, master)
	// add a new line to be sure
	if err := write(t, master, 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// the standby2 should be readded to the cluster and a new db assigned
	// (since standby1 is dead) and then synced to the master db
	waitKeeperReady(t, sm, standby2)
	if err := waitLines(t, standby2, 2, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standby2.WaitDBRole(common.RoleStandby, nil, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// wait for standby1 keeper to be removed from the cluster data since it's dead a without an assigned db
	if err := WaitClusterDataKeepers([]string{master.uid, standby2.uid}, sm, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
