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
		InitMode:           cluster.ClusterInitModeNew,
		FailInterval:       cluster.Duration{Duration: 10 * time.Second},
		ConvergenceTimeout: cluster.Duration{Duration: 30 * time.Second},
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
		tks[tk.id] = tk
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
		tss[ts.id] = ts
	}

	defer shutdown(tks, tss, tstore)

	// Wait for clusterView containing a master
	masterUID, err := WaitClusterDataWithMaster(sm, 30*time.Second)
	if err != nil {
		t.Fatal("expected a master in cluster view")
	}
	if err := tks[masterUID].WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func setupServers(t *testing.T, clusterName, dir string, numKeepers, numSentinels uint8, syncRepl bool, usePgrewind bool) (testKeepers, testSentinels, *TestStore) {
	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:               cluster.ClusterInitModeNew,
		SleepInterval:          cluster.Duration{Duration: 2 * time.Second},
		FailInterval:           cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout:     cluster.Duration{Duration: 30 * time.Second},
		SynchronousReplication: syncRepl,
		UsePgrewind:            usePgrewind,
		PGParameters:           make(cluster.PGParameters),
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
		tss[ts.id] = ts
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
		tks[tk.id] = tk
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

func waitMasterStandbysReady(t *testing.T, sm *store.StoreManager, tks testKeepers) (master *TestKeeper, standbys []*TestKeeper, err error) {
	// Wait for normal cluster phase (master ready)
	masterUID, err := WaitClusterDataWithMaster(sm, 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	master = tks[masterUID]
	// Return all other keepers as standbys (assume that MaxStandbysPerSender is greater the the number of keepers)
	for _, tk := range tks {
		if tk.id == masterUID {
			continue
		}
		standbys = append(standbys, tk)
	}
	if err := master.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, standby := range standbys {
		if err := standby.WaitDBUp(60 * time.Second); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
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

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, false)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys, err := waitMasterStandbysReady(t, sm, tks)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
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
	if err := waitLines(t, standbys[0], 1, 10*time.Second); err != nil {
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

func testFailover(t *testing.T, syncRepl bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, false)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys, err := waitMasterStandbysReady(t, sm, tks)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop the keeper process on master, should also stop the database
	t.Logf("Stopping current master keeper: %s", master.id)
	master.Stop()

	if err := standbys[0].WaitRole(common.RoleMaster, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}
}

func TestFailover(t *testing.T) {
	t.Parallel()
	testFailover(t, false)
}

func TestFailoverSyncRepl(t *testing.T) {
	t.Parallel()
	testFailover(t, true)
}

// Tests standby elected as new master but fails to become master. Then old
// master comes back and is re-elected as master.
func testFailoverFailed(t *testing.T, syncRepl bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, false)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys, err := waitMasterStandbysReady(t, sm, tks)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	standby := standbys[0]

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop the keeper process on master, should also stop the database
	t.Logf("Stopping current master keeper: %s", master.id)
	master.Stop()

	// Wait for cluster data containing standby as master
	if err := WaitClusterDataMaster(standby.id, sm, 30*time.Second); err != nil {
		t.Fatalf("expected master %q in cluster view", standby.id)
	}

	// Stopping standby before reading the new cluster data and promoting
	// TODO(sgotti) this is flacky and the standby can read the data and
	// publish new state before it's stopped
	t.Logf("Stopping current stanby keeper: %s", master.id)
	standby.Stop()

	t.Logf("Starting previous master keeper: %s", master.id)
	master.Start()
	// Wait for cluster data containing previous master as master
	if err := WaitClusterDataMaster(master.id, sm, 30*time.Second); err != nil {
		t.Fatalf("expected master %q in cluster view", master.id)
	}
}

func TestFailoverFailed(t *testing.T) {
	t.Parallel()
	testFailoverFailed(t, false)
}
func TestFailoverFailedSyncRepl(t *testing.T) {
	t.Parallel()
	testFailoverFailed(t, true)
}

func testOldMasterRestart(t *testing.T, syncRepl, usePgrewind bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, usePgrewind)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys, err := waitMasterStandbysReady(t, sm, tks)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop the keeper process on master, should also stop the database
	t.Logf("Stopping current master keeper: %s", master.id)
	master.Stop()

	if err := standbys[0].WaitRole(common.RoleMaster, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	if err := write(t, standbys[0], 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Restart the old master
	t.Logf("Restarting old master keeper: %s", master.id)
	if err := master.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Wait old master synced with standby
	if err := waitLines(t, master, 2, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := master.WaitRole(common.RoleStandby, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestOldMasterRestart(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, false, false)
}

func TestOldMasterRestartSyncRepl(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, true, false)
}

func TestOldMasterRestartPgrewind(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, false, true)
}

func TestOldMasterRestartSyncReplPgrewind(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, true, true)
}

func testPartition1(t *testing.T, syncRepl, usePgrewind bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, syncRepl, usePgrewind)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys, err := waitMasterStandbysReady(t, sm, tks)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Freeze the keeper and postgres processes on the master
	t.Logf("SIGSTOPping current master keeper: %s", master.id)
	if err := master.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("SIGSTOPping current master postgres: %s", master.id)
	if err := master.SignalPG(syscall.SIGSTOP); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[0].WaitRole(common.RoleMaster, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	if err := write(t, standbys[0], 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Make the master come back
	t.Logf("Resuming old master keeper: %s", master.id)
	if err := master.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("Resuming old master postgres: %s", master.id)
	if err := master.SignalPG(syscall.SIGCONT); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Wait replicated data to old master
	if err := waitLines(t, master, 2, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := master.WaitRole(common.RoleStandby, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestPartition1(t *testing.T) {
	t.Parallel()
	testPartition1(t, false, false)
}

func TestPartition1SyncRepl(t *testing.T) {
	t.Parallel()
	testPartition1(t, true, false)
}

func TestPartition1Pgrewind(t *testing.T) {
	t.Parallel()
	testPartition1(t, false, true)
}

func TestPartition1SyncReplPgrewind(t *testing.T) {
	t.Parallel()
	testPartition1(t, true, true)
}

func testTimelineFork(t *testing.T, syncRepl, usePgrewind bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 3, 1, syncRepl, usePgrewind)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys, err := waitMasterStandbysReady(t, sm, tks)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Wait replicated data to standby
	if err := waitLines(t, standbys[0], 1, 10*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Stop one standby
	t.Logf("Stopping standby[0]: %s", standbys[0].id)
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
	t.Logf("Stopping master: %s", master.id)
	master.Stop()
	t.Logf("Stopping standby[1]: %s", standbys[1].id)
	standbys[1].Stop()
	if err := master.WaitDBDown(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[1].WaitDBDown(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Start standby[0]. It will be elected as master but it'll be behind (having only one line).
	t.Logf("Starting standby[0]: %s", standbys[0].id)
	standbys[0].Start()
	if err := standbys[0].WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[0].WaitRole(common.RoleMaster, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err := getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	// Start the other standby, it should be ahead of current on previous timeline and should full resync himself
	t.Logf("Starting standby[1]: %s", standbys[1].id)
	standbys[1].Start()
	// Standby[1] will start, then it'll detect it's in another timelinehistory,
	// will stop, full resync and start. We have to avoid detecting it up
	// at the first start. Do this waiting for the number of expected lines.
	if err := waitLines(t, standbys[1], 1, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[1].WaitRole(common.RoleStandby, 60*time.Second); err != nil {
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
func TestMasterChangedAddress(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, tstore := setupServers(t, clusterName, dir, 2, 1, false, false)
	defer shutdown(tks, tss, tstore)

	storePath := filepath.Join(common.StoreBasePath, clusterName)
	sm := store.NewStoreManager(tstore.store, storePath)

	master, standbys, err := waitMasterStandbysReady(t, sm, tks)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := populate(t, master); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, master, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Wait standby synced with master
	if err := waitLines(t, master, 1, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Restart the keeper process on master with new keeper and postgres
	// addresses (in this case only the port is changed)
	t.Logf("Restarting current master keeper %q with different addresses", master.id)
	master.Stop()
	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)
	master, err = NewTestKeeperWithID(t, dir, master.id, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	tks[master.id] = master

	if err := master.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := master.WaitRole(common.RoleMaster, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := write(t, master, 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Wait standby synced to master with changed address
	if err := waitLines(t, standbys[0], 2, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
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
		InitMode:             cluster.ClusterInitModeNew,
		SleepInterval:        cluster.Duration{Duration: 2 * time.Second},
		FailInterval:         cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout:   cluster.Duration{Duration: 30 * time.Second},
		MaxStandbysPerSender: 1,
		PGParameters:         make(cluster.PGParameters),
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
	if err := master.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
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
	t.Logf("Stopping current standby keeper: %s", standby.id)
	standby.Stop()

	// Wait for other keeper to have a standby db assigned
	var newStandby *TestKeeper
	for _, tk := range tks {
		if tk.id != master.id && tk.id != standby.id {
			newStandby = tk
		}
	}

	if err := WaitStandbyKeeper(sm, newStandby.id, 20*time.Second); err != nil {
		t.Fatalf("expected keeper %s to have a standby db assigned: %v", newStandby.id, err)
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
		InitMode:             cluster.ClusterInitModeNew,
		SleepInterval:        cluster.Duration{Duration: 2 * time.Second},
		FailInterval:         cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout:   cluster.Duration{Duration: 30 * time.Second},
		MaxStandbysPerSender: 2,
		PGParameters:         make(cluster.PGParameters),
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
	if err := master.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
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
