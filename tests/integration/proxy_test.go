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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/common"
	"github.com/sorintlab/stolon/internal/store"

	"github.com/satori/go.uuid"
)

func TestProxyListening(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	tstore, err := NewTestStore(t, dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	tp, err := NewTestProxy(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tp.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tp.Stop()

	t.Logf("test proxy start with store down. Should not listen")
	// tp should not listen because it cannot talk with store
	if err := tp.WaitNotListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp not listening due to failed store communication, but it's listening.")
	}

	tp.Stop()

	if err := tstore.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tstore.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store up: %v", err)
	}
	defer func() {
		if tstore.cmd != nil {
			tstore.Stop()
		}
	}()

	storePath := filepath.Join(common.StorePrefix, clusterName)

	sm := store.NewKVBackedStore(tstore.store, storePath)

	cd := &cluster.ClusterData{
		FormatVersion: cluster.CurrentCDFormatVersion,
		Cluster: &cluster.Cluster{
			UID:        "01",
			Generation: 1,
			Spec: &cluster.ClusterSpec{
				InitMode:     cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
				FailInterval: &cluster.Duration{Duration: 10 * time.Second},
			},
			Status: cluster.ClusterStatus{
				CurrentGeneration: 1,
				Phase:             cluster.ClusterPhaseNormal,
				Master:            "01",
			},
		},
		Keepers: cluster.Keepers{
			"01": &cluster.Keeper{
				UID:  "01",
				Spec: &cluster.KeeperSpec{},
				Status: cluster.KeeperStatus{
					Healthy: true,
				},
			},
		},
		DBs: cluster.DBs{
			"01": &cluster.DB{
				UID:        "01",
				Generation: 1,
				ChangeTime: time.Time{},
				Spec: &cluster.DBSpec{
					KeeperUID: "01",
					Role:      common.RoleMaster,
					Followers: []string{"02"},
				},
				Status: cluster.DBStatus{
					Healthy:           false,
					CurrentGeneration: 1,
				},
			},
		},
		Proxy: &cluster.Proxy{
			Spec: cluster.ProxySpec{
				MasterDBUID: "01",
			},
		},
	}
	pair, err := sm.AtomicPutClusterData(context.TODO(), cd, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// test proxy start with the store up
	t.Logf("test proxy start with the store up. Should listen")
	if err := tp.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test proxy error communicating with store. Should stop listening")
	// Stop store
	tstore.Stop()
	if err := tstore.WaitDown(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store down: %v", err)
	}

	// tp should not listen because it cannot talk with the store
	if err := tp.WaitNotListening(cluster.DefaultProxyTimeout * 2); err != nil {
		t.Fatalf("expecting tp not listening due to failed store communication, but it's listening.")
	}

	t.Logf("test proxy communication with store restored. Should start listening")
	// Start store
	if err := tstore.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tstore.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store up: %v", err)
	}
	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test proxy error communicating with store but restored before proxy check timeout. Should continue listening")
	// Stop store
	tstore.Stop()
	if err := tstore.WaitDown(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store down: %v", err)
	}
	// wait less than DefaultProxyTimeout
	time.Sleep(cluster.DefaultProxyTimeout / 3)
	// Start store
	if err := tstore.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tstore.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on store up: %v", err)
	}
	// tp should listen
	if ok := tp.CheckListening(); !ok {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}
	// wait proxy reading again from the store
	time.Sleep(2 * cluster.DefaultProxyCheckInterval)
	// tp should listen
	if ok := tp.CheckListening(); !ok {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test proxyConf removed. Should continue listening")
	// remove proxyConf
	cd.Proxy.Spec.MasterDBUID = ""
	pair, err = sm.AtomicPutClusterData(context.TODO(), cd, pair)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test proxyConf restored. Should continue listening")
	// Set proxyConf again
	cd.Proxy.Spec.MasterDBUID = "01"
	pair, err = sm.AtomicPutClusterData(context.TODO(), cd, pair)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	t.Logf("test clusterView removed. Should continue listening")
	// remove whole clusterview
	_, err = sm.AtomicPutClusterData(context.TODO(), nil, pair)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	// simulate the store in hang by freezing its process
	t.Logf("SIGSTOPping the store: %s", tstore.uid)
	if err := tstore.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should not listen because it cannot talk with the store
	if err := tp.WaitNotListening(cluster.DefaultProxyTimeout * 2); err != nil {
		t.Fatalf("expecting tp not listening due to failed store communication, but it's listening.")
	}

	// resume the store
	t.Logf("SIGCONTing the store: %s", tstore.uid)
	if err := tstore.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}
}
