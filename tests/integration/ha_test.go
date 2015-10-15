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
	"syscall"
	"testing"
	"time"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/common"
)

func setupServers(t *testing.T, dir string, numKeepers, numSentinels uint8) ([]*TestKeeper, []*TestSentinel) {
	cluster := uuid.NewV4().String()

	tms := []*TestKeeper{}
	tss := []*TestSentinel{}

	tm, err := NewTestKeeper(dir, cluster)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	tms = append(tms, tm)

	fmt.Printf("tm: %v\n", tm)

	// Start sentinels
	for i := uint8(0); i < numSentinels; i++ {
		ts, err := NewTestSentinel(dir, cluster)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := ts.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tss = append(tss, ts)
	}
	if err := tm.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tm.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tm.WaitRole(common.MasterRole, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Start standbys
	for i := uint8(1); i < numKeepers; i++ {
		tm, err := NewTestKeeper(dir, cluster)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tm.Start(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tm.WaitDBUp(60 * time.Second); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if err := tm.WaitRole(common.StandbyRole, 30*time.Second); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		tms = append(tms, tm)
	}
	return tms, tss
}

func populate(t *testing.T, tm *TestKeeper) error {
	_, err := tm.Exec("CREATE TABLE table01(ID INT PRIMARY KEY NOT NULL, VALUE INT NOT NULL)")
	return err
}

func write(t *testing.T, tm *TestKeeper, id, value int) error {
	_, err := tm.Exec("INSERT INTO table01 VALUES ($1, $2)", id, value)
	return err
}

func getLines(t *testing.T, tm *TestKeeper) (int, error) {
	rows, err := tm.Query("SELECT FROM table01")
	if err != nil {
		return 0, err
	}
	c := 0
	for rows.Next() {
		c++
	}
	return c, rows.Err()
}

func waitLines(t *testing.T, tm *TestKeeper, num int, timeout time.Duration) error {
	start := time.Now()
	c := -1
	for time.Now().Add(-timeout).Before(start) {
		c, err := getLines(t, tm)
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

func shutdown(tms []*TestKeeper, tss []*TestSentinel) {
	for _, ts := range tss {
		if ts.cmd != nil {
			ts.Stop()
		}
	}
	for _, tm := range tms {
		if tm.cmd != nil {
			tm.SignalPG(os.Kill)
			tm.Signal(os.Kill)
		}
	}
}

func getRoles(t *testing.T, tms []*TestKeeper) (master *TestKeeper, standbys []*TestKeeper, err error) {
	for _, tm := range tms {
		ok, err := tm.IsMaster()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if ok {
			master = tm
		} else {
			standbys = append(standbys, tm)
		}
	}
	return
}

func TestMasterStandby(t *testing.T) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tms, tss := setupServers(t, dir, 2, 1)
	defer shutdown(tms, tss)

	master, standbys, err := getRoles(t, tms)
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

func TestFailover(t *testing.T) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tms, tss := setupServers(t, dir, 2, 1)
	defer shutdown(tms, tss)

	master, standbys, err := getRoles(t, tms)
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

func TestOldMasterRestart(t *testing.T) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tms, tss := setupServers(t, dir, 2, 1)
	defer shutdown(tms, tss)

	master, standbys, err := getRoles(t, tms)
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

func TestPartition1(t *testing.T) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tms, tss := setupServers(t, dir, 2, 1)
	defer shutdown(tms, tss)

	master, standbys, err := getRoles(t, tms)
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

func TestTimelineFork(t *testing.T) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tms, tss := setupServers(t, dir, 3, 1)
	defer shutdown(tms, tss)

	master, standbys, err := getRoles(t, tms)
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
