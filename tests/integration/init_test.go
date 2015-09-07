package integration

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
)

func TestInit(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)
	cluster := uuid.NewV4().String()
	tm, err := NewTestKeeper(dir, cluster)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("tm: %v", tm)

	err = tm.Start()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	err = tm.WaitDBUp(60 * time.Second)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("database is up")

	tm.Stop()
}
