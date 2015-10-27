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
	clusterName := uuid.NewV4().String()

	ts, err := NewTestSentinel(dir, clusterName)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := ts.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer ts.Stop()
	tm, err := NewTestKeeper(dir, clusterName)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("tm: %v", tm)

	if err := tm.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tm.Stop()

	if err := tm.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	t.Logf("database is up")

}

func TestExclusiveLock(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)
	clusterName := uuid.NewV4().String()

	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	tk1, err := NewTestKeeperWithID(dir, id, clusterName)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk1.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk1.Stop()

	// Wait for tk1 up before starting tk2
	if err := tk1.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("expecting tk1 up but it's down")
	}

	tk2, err := NewTestKeeperWithID(dir, id, clusterName)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk2.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk2.Stop()

	// tk2 should exit because it cannot take an exclusive lock on dataDir
	if err := tk2.WaitProcess(10 * time.Second); err != nil {
		t.Fatalf("expecting tk2 exiting due to failed exclusive lock, but it's active.")
	}

}
