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
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	etcdm "github.com/sorintlab/stolon/pkg/etcd"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
)

func TestProxyListening(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	clusterName := uuid.NewV4().String()

	te, err := NewTestEtcd(dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	etcdEndpoints := fmt.Sprintf("http://%s:%s", te.listenAddress, te.port)

	tp, err := NewTestProxy(dir, clusterName, etcdEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tp.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tp.Stop()

	log.Printf("test proxy start with etcd down. Should not listen")
	// tp should not listen because it cannot talk with etcd
	if err := tp.WaitNotListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp not listening due to failed etcd communication, but it's listening.")
	}

	tp.Stop()

	if err := te.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := te.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on etcd up: %v", err)
	}
	defer func() {
		if te.cmd != nil {
			te.Stop()
		}
	}()

	etcdPath := filepath.Join(common.EtcdBasePath, clusterName)
	e, err := etcdm.NewEtcdManager(etcdEndpoints, etcdPath, common.DefaultEtcdRequestTimeout)
	if err != nil {
		t.Fatalf("cannot create etcd manager: %v", err)
	}

	res, err := e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:      cluster.DurationP(5 * time.Second),
				KeeperFailInterval: cluster.DurationP(10 * time.Second),
			},
			ProxyConf: &cluster.ProxyConf{
				// fake pg address, not relevant
				Host: "localhost",
				Port: "5432",
			},
		}, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	prevCDIndex := res.Node.ModifiedIndex

	// test etcd start with etcd up
	log.Printf("test proxy start with etcd up. Should listen")
	if err := tp.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	log.Printf("test proxy error communicating with etcd. Should stop listening")
	// Stop etcd
	te.Stop()
	if err := te.WaitDown(10 * time.Second); err != nil {
		t.Fatalf("error waiting on etcd down: %v", err)
	}

	// tp should not listen because it cannot talk with etcd
	if err := tp.WaitNotListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp not listening due to failed etcd communication, but it's listening.")
	}

	log.Printf("test proxy communication with etcd restored. Should start listening")
	// Start etcd
	if err := te.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := te.WaitUp(10 * time.Second); err != nil {
		t.Fatalf("error waiting on etcd up: %v", err)
	}
	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	log.Printf("test proxyConf removed. Should continue listening")
	// remove proxyConf
	res, err = e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:      cluster.DurationP(5 * time.Second),
				KeeperFailInterval: cluster.DurationP(10 * time.Second),
			},
			ProxyConf: nil,
		}, prevCDIndex)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	prevCDIndex = res.Node.ModifiedIndex

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	log.Printf("test proxyConf restored. Should continue listening")
	// Set proxyConf again
	res, err = e.SetClusterData(cluster.KeepersState{},
		&cluster.ClusterView{
			Version: 1,
			Config: &cluster.NilConfig{
				SleepInterval:      cluster.DurationP(5 * time.Second),
				KeeperFailInterval: cluster.DurationP(10 * time.Second),
			},
			ProxyConf: &cluster.ProxyConf{
				// fake pg address, not relevant
				Host: "localhost",
				Port: "5432",
			},
		}, prevCDIndex)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	prevCDIndex = res.Node.ModifiedIndex

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}

	log.Printf("test clusterView removed. Should continue listening")
	// remove whole clusterview
	_, err = e.SetClusterData(cluster.KeepersState{}, nil, prevCDIndex)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// tp should listen
	if err := tp.WaitListening(10 * time.Second); err != nil {
		t.Fatalf("expecting tp listening, but it's not listening.")
	}
}
