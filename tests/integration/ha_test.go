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

	// Start slaves
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
		if err := tm.WaitRole(common.SlaveRole, 30*time.Second); err != nil {
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
		t.Fatalf("unexpected err: %v", err)
	}
	c := 0
	for rows.Next() {
		c++
	}
	return c, rows.Err()
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

func getRoles(t *testing.T, tms []*TestKeeper) (master *TestKeeper, slaves []*TestKeeper, err error) {
	for _, tm := range tms {
		ok, err := tm.IsMaster()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if ok {
			master = tm
		} else {
			slaves = append(slaves, tm)
		}
	}
	return
}

func TestMasterSlave(t *testing.T) {
	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tms, tss := setupServers(t, dir, 2, 1)
	defer shutdown(tms, tss)

	master, slaves, err := getRoles(t, tms)
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
		t.Fatalf("wrong number of lines, want: %s, got: %d", 1, c)
	}
	time.Sleep(1 * time.Second)
	c, err = getLines(t, slaves[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %s, got: %d", 1, c)
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

	master, slaves, err := getRoles(t, tms)
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

	if err := slaves[0].WaitRole(common.MasterRole, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, slaves[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %s, got: %d", 1, c)
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

	master, slaves, err := getRoles(t, tms)
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

	if err := slaves[0].WaitRole(common.MasterRole, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, slaves[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %s, got: %d", 1, c)
	}

	if err := write(t, slaves[0], 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Restart the old master
	fmt.Printf("Restarting old master keeper: %s\n", master.id)
	if err := master.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := master.WaitRole(common.SlaveRole, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err = getLines(t, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 2 {
		t.Fatalf("wrong number of lines, want: %s, got: %d", 2, c)
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

	master, slaves, err := getRoles(t, tms)
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
	if err := slaves[0].WaitRole(common.MasterRole, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	c, err := getLines(t, slaves[0])
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %s, got: %d", 1, c)
	}

	if err := write(t, slaves[0], 2, 2); err != nil {
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
	if err := master.WaitRole(common.SlaveRole, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err = getLines(t, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 2 {
		t.Fatalf("wrong number of lines, want: %s, got: %d", 2, c)
	}
}
