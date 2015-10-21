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
