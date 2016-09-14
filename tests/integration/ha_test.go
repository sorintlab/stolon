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

	kvstore, err := store.NewStore(tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("cannot create store: %v", err)
	}

	e := store.NewStoreManager(kvstore, storePath)

	// TODO(sgotti) change this to a call to the sentinel to change the
	// cluster config (when the sentinel's code is done)
	e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:           &cluster.Duration{5 * time.Second},
				KeeperFailInterval:      &cluster.Duration{10 * time.Second},
				InitWithMultipleKeepers: cluster.BoolP(true),
			},
		}, nil)

	tks := []*TestKeeper{}
	tss := []*TestSentinel{}

	// Start 3 keepers
	for i := uint8(0); i < 3; i++ {
		tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tk.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tks = append(tks, tk)
		if err := tk.WaitDBUp(60 * time.Second); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	}

	// Start 2 sentinels
	for i := uint8(0); i < 2; i++ {
		ts, err := NewTestSentinel(t, dir, clusterName, tstore.storeBackend, storeEndpoints)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := ts.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tss = append(tss, ts)
	}

	defer shutdown(tks, tss, tstore)

	// Wait for clusterView containing a master
	if err := WaitClusterViewWithMaster(e, 30*time.Second); err != nil {
		t.Fatal("expected a master in cluster view")
	}
}

func setupServers(t *testing.T, clusterName, dir string, numKeepers, numSentinels uint8, syncRepl bool, usePGRewind bool) ([]*TestKeeper, []*TestSentinel, *TestStore) {
	tstore := setupStore(t, dir)

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	storePath := filepath.Join(common.StoreBasePath, clusterName)

	kvstore, err := store.NewStore(tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("cannot create store: %v", err)
	}

	e := store.NewStoreManager(kvstore, storePath)

	// TODO(sgotti) change this to a call to the sentinel to change the
	// cluster config (when the sentinel's code is done)
	e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:          &cluster.Duration{5 * time.Second},
				KeeperFailInterval:     &cluster.Duration{10 * time.Second},
				SynchronousReplication: cluster.BoolP(syncRepl),
				UsePGRewind:            cluster.BoolP(usePGRewind),
			},
		}, nil)

	tks := []*TestKeeper{}
	tss := []*TestSentinel{}

	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tks = append(tks, tk)

	t.Logf("tk: %v", tk)

	// Start sentinels
	for i := uint8(0); i < numSentinels; i++ {
		ts, err := NewTestSentinel(t, dir, clusterName, tstore.storeBackend, storeEndpoints)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := ts.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tss = append(tss, ts)
	}

	// Start first keeper
	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitRole(common.MasterRole, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Wait for clusterView containing tk as master
	if err := WaitClusterViewMaster(tk.id, e, 30*time.Second); err != nil {
		t.Fatalf("expected master %q in cluster view", tk.id)
	}

	// Start other keepers
	for i := uint8(1); i < numKeepers; i++ {
		tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tk.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tk.WaitDBUp(60 * time.Second); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// Wait for clusterView containing tk as standby
		if err := tk.WaitRole(common.StandbyRole, 30*time.Second); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tks = append(tks, tk)
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

func shutdown(tks []*TestKeeper, tss []*TestSentinel, tstore *TestStore) {
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

func getRoles(t *testing.T, tks []*TestKeeper) (master *TestKeeper, standbys []*TestKeeper, err error) {
	for _, tk := range tks {
		ok, err := tk.IsMaster()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if ok {
			master = tk
		} else {
			standbys = append(standbys, tk)
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

	tks, tss, te := setupServers(t, clusterName, dir, 2, 1, syncRepl, false)
	defer shutdown(tks, tss, te)

	master, standbys, err := getRoles(t, tks)
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

	tks, tss, te := setupServers(t, clusterName, dir, 2, 1, syncRepl, false)
	defer shutdown(tks, tss, te)

	master, standbys, err := getRoles(t, tks)
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

	if err := standbys[0].WaitRole(common.MasterRole, 30*time.Second); err != nil {
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

func testOldMasterRestart(t *testing.T, syncRepl, usePGRewind bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, te := setupServers(t, clusterName, dir, 2, 1, syncRepl, usePGRewind)
	defer shutdown(tks, tss, te)

	master, standbys, err := getRoles(t, tks)
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

	if err := standbys[0].WaitRole(common.MasterRole, 30*time.Second); err != nil {
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
	if err := master.WaitRole(common.StandbyRole, 30*time.Second); err != nil {
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

func TestOldMasterRestartPGRewind(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, false, true)
}

func TestOldMasterRestartSyncReplPGRewind(t *testing.T) {
	t.Parallel()
	testOldMasterRestart(t, true, true)
}

func testPartition1(t *testing.T, syncRepl, usePGRewind bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, te := setupServers(t, clusterName, dir, 2, 1, syncRepl, usePGRewind)
	defer shutdown(tks, tss, te)

	master, standbys, err := getRoles(t, tks)
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
	if err := standbys[0].WaitRole(common.MasterRole, 60*time.Second); err != nil {
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
	if err := master.WaitRole(common.StandbyRole, 60*time.Second); err != nil {
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

func TestPartition1PGRewind(t *testing.T) {
	t.Parallel()
	testPartition1(t, false, true)
}

func TestPartition1SyncReplPGRewind(t *testing.T) {
	t.Parallel()
	testPartition1(t, true, true)
}

func testTimelineFork(t *testing.T, syncRepl, usePGRewind bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tks, tss, te := setupServers(t, clusterName, dir, 3, 1, syncRepl, usePGRewind)
	defer shutdown(tks, tss, te)

	master, standbys, err := getRoles(t, tks)
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
	t.Logf("Stopping standby[0]: %s", master.id)
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
	if err := standbys[0].WaitRole(common.MasterRole, 60*time.Second); err != nil {
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
	if err := standbys[1].WaitRole(common.StandbyRole, 60*time.Second); err != nil {
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

func TestTimelineForkPGRewind(t *testing.T) {
	t.Parallel()
	testTimelineFork(t, false, true)
}

func TestTimelineForkSyncReplPGRewind(t *testing.T) {
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

	master, standbys, err := getRoles(t, tks)
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
	tks = append(tks, master)

	if err := master.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := master.WaitRole(common.MasterRole, 30*time.Second); err != nil {
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
