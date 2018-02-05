// Copyright 2017 Sorint.lab
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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/store"
)

func TestSentinelEnabledProxies(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	tstore := setupStore(t, dir)
	defer tstore.Stop()

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	clusterName := uuid.NewV4().String()

	storePath := filepath.Join(common.StorePrefix, clusterName)
	sm := store.NewStore(tstore.store, storePath)

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		PGParameters:       defaultPGParameters,
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
	defer ts.Stop()
	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk.Stop()

	tp, err := NewTestProxy(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tp.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	waitKeeperReady(t, sm, tk)

	// the proxy should connect to the right master
	if err := tp.WaitRightMaster(tk, 3*cluster.DefaultProxyCheckInterval); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	t.Logf("stopping sentinel")
	ts.Stop()

	cd, _, err := sm.GetClusterData(context.TODO())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	enabledProxies := cd.Proxy.Spec.EnabledProxies

	t.Logf("starting sentinel")
	ts.Start()

	// check that the sentinel has become leader (cluster data is changed since
	// it's updating keepers status) TODO(sgotti) find a better way to determine
	// if the sentinel is the leader and is updating the clusterdata
	if err := WaitClusterDataUpdated(sm, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	t.Logf("clusterdata changed")

	// check that the enabled proxies aren't changed (same Proxy Generation and same EnabledProxies)
	cd, _, err = sm.GetClusterData(context.TODO())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !reflect.DeepEqual(enabledProxies, cd.Proxy.Spec.EnabledProxies) {
		t.Fatalf("expected enabled proxies: %q, got: %q", enabledProxies, cd.Proxy.Spec.EnabledProxies)
	}

	// add another proxy
	tp2, err := NewTestProxy(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tp2.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// the proxy should connect to the right master
	if err := tp2.WaitRightMaster(tk, 3*cluster.DefaultProxyCheckInterval); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := WaitClusterDataEnabledProxiesNum(sm, 2, 3*initialClusterSpec.SleepInterval.Duration); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// freeze the proxy
	t.Logf("SIGSTOPping proxy: %s", tp2.uid)
	if err := tp2.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := WaitClusterDataEnabledProxiesNum(sm, 1, 3*cluster.DefaultProxyTimeoutInterval); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// unfreeze the proxy
	t.Logf("SIGCONTing proxy: %s", tp2.uid)
	if err := tp2.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := WaitClusterDataEnabledProxiesNum(sm, 2, 6*initialClusterSpec.SleepInterval.Duration); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
