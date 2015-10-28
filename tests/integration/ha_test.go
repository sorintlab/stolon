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

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	etcdm "github.com/sorintlab/stolon/pkg/etcd"
)

func setupServers(t *testing.T, dir string, numKeepers, numSentinels uint8, syncRepl bool) ([]*TestKeeper, []*TestSentinel, *TestEtcd) {
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

	clusterName := uuid.NewV4().String()

	etcdPath := filepath.Join(common.EtcdBasePath, clusterName)
	e, err := etcdm.NewEtcdManager(etcdEndpoints, etcdPath, common.DefaultEtcdRequestTimeout)
	if err != nil {
		t.Fatalf("cannot create etcd manager: %v", err)
	}
	// TODO(sgotti) change this to a call to the sentinel to change the
	// cluster config (when the sentinel's code is done)
	e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:          cluster.DurationP(5 * time.Second),
				KeeperFailInterval:     cluster.DurationP(10 * time.Second),
				SynchronousReplication: cluster.BoolP(syncRepl),
			},
		}, 0)

	tks := []*TestKeeper{}
	tss := []*TestSentinel{}

	tk, err := NewTestKeeper(dir, clusterName, etcdEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tks = append(tks, tk)

	fmt.Printf("tk: %v\n", tk)

	// Start sentinels
	for i := uint8(0); i < numSentinels; i++ {
		ts, err := NewTestSentinel(dir, clusterName, etcdEndpoints)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := ts.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tss = append(tss, ts)
	}
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

	// Start standbys
	for i := uint8(1); i < numKeepers; i++ {
		tk, err := NewTestKeeper(dir, clusterName, etcdEndpoints)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tk.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tk.WaitDBUp(60 * time.Second); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tk.WaitRole(common.StandbyRole, 30*time.Second); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tks = append(tks, tk)
	}
	return tks, tss, te
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
	return fmt.Errorf("timeout with wrong number of lines: %d", c)

}

func shutdown(tks []*TestKeeper, tss []*TestSentinel, te *TestEtcd) {
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
	if te.cmd != nil {
		te.Kill()
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

	tks, tss, te := setupServers(t, dir, 2, 1, syncRepl)
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
	// TODO(sgotti) do not sleep but make a loop
	time.Sleep(1 * time.Second)
	c, err = getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

}

func TestMasterStandbySyncRepl(t *testing.T) {
	testMasterStandby(t, false)
}

func TestMasterStandby(t *testing.T) {
	testMasterStandby(t, true)
}

func testFailover(t *testing.T, syncRepl bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tks, tss, te := setupServers(t, dir, 2, 1, syncRepl)
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
	fmt.Printf("Stopping current master keeper: %s\n", master.id)
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
	testFailover(t, false)
}
func TestFailoverSyncRepl(t *testing.T) {
	testFailover(t, true)
}

func testOldMasterRestart(t *testing.T, syncRepl bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tks, tss, te := setupServers(t, dir, 2, 1, syncRepl)
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
	fmt.Printf("Stopping current master keeper: %s\n", master.id)
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
	fmt.Printf("Restarting old master keeper: %s\n", master.id)
	if err := master.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := master.WaitRole(common.StandbyRole, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err = getLines(t, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 2 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 2, c)
	}
}

func TestOldMasterRestart(t *testing.T) {
	testOldMasterRestart(t, false)
}

func TestOldMasterRestartSyncRepl(t *testing.T) {
	testOldMasterRestart(t, true)
}

func testPartition1(t *testing.T, syncRepl bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tks, tss, te := setupServers(t, dir, 2, 1, syncRepl)
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
	fmt.Printf("SIGSTOPping current master keeper: %s\n", master.id)
	if err := master.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	fmt.Printf("SIGSTOPping current master postgres: %s\n", master.id)
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
	fmt.Printf("Resuming old master keeper: %s\n", master.id)
	if err := master.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	fmt.Printf("Resuming old master postgres: %s\n", master.id)
	if err := master.SignalPG(syscall.SIGCONT); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := master.WaitRole(common.StandbyRole, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err = getLines(t, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 2 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 2, c)
	}
}

func TestPartition1(t *testing.T) {
	testPartition1(t, false)
}

func TestPartition1SyncRepl(t *testing.T) {
	testPartition1(t, true)
}

func testTimelineFork(t *testing.T, syncRepl bool) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tks, tss, te := setupServers(t, dir, 3, 1, syncRepl)
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

	// Stop one standby
	fmt.Printf("Stopping standby[0]: %s\n", master.id)
	standbys[0].Stop()
	if err := standbys[0].WaitDBDown(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Write to master (and replicated to remaining standby)
	if err := write(t, master, 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Wait replicated data to standby
	// TODO(sgotti) do not sleep but make a loop
	time.Sleep(1 * time.Second)
	c, err := getLines(t, standbys[1])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 2 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	// Stop the master and remaining standby
	master.Stop()
	standbys[1].Stop()
	if err := master.WaitDBDown(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[1].WaitDBDown(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Start standby[0]. It will be elected as master but it'll be behind (having only one line).
	fmt.Printf("Starting standby[0]: %s\n", standbys[0].id)
	standbys[0].Start()
	if err := standbys[0].WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[0].WaitRole(common.MasterRole, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err = getLines(t, standbys[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 1, c)
	}

	// Start the other stadby, it should be ahead of current on previous timeline and should full resync himself
	fmt.Printf("Starting standby[1]: %s\n", standbys[1].id)
	standbys[1].Start()
	// Standby[1] will start, then it'll detect it's in another timelinehistory,
	// will stop, full resync and start. We have to avoid detecting it up
	// at the first start.
	// Wait for the number of expected lines.
	if err := waitLines(t, standbys[1], 1, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := standbys[1].WaitRole(common.StandbyRole, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestTimelineFork(t *testing.T) {
	testTimelineFork(t, false)
}

func TestTimelineForkSyncRepl(t *testing.T) {
	testTimelineFork(t, true)
}
