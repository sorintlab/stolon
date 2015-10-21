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

package etcd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/pkg/capnslog"
	"github.com/sorintlab/stolon/pkg/cluster"

	etcd "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/etcd/client"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/fleet/pkg/lease"
)

var log = capnslog.NewPackageLogger("github.com/sorintlab/stolon/pkg", "etcd")

const (
	configFile       = "config"
	discoveryInfoDir = "discovery"
	clusterDataFile  = "clusterdata"
	proxyViewFile    = "proxyview"
)

type EtcdManager struct {
	etcdPath       string
	eCfg           etcd.Config
	kAPI           etcd.KeysAPI
	requestTimeout time.Duration
}

func NewEtcdManager(etcdEndpoints string, path string, requestTimeout time.Duration) (*EtcdManager, error) {
	eCfg := etcd.Config{
		Transport: &http.Transport{},
		Endpoints: strings.Split(etcdEndpoints, ","),
	}
	eClient, err := etcd.New(eCfg)
	if err != nil {
		return nil, err
	}
	kAPI := etcd.NewKeysAPI(eClient)

	return &EtcdManager{
		etcdPath:       path,
		eCfg:           eCfg,
		kAPI:           kAPI,
		requestTimeout: requestTimeout,
	}, nil
}

func (e *EtcdManager) NewLeaseManager() lease.Manager {
	return lease.NewEtcdLeaseManager(e.kAPI, e.etcdPath, e.requestTimeout)
}

func (e *EtcdManager) SetClusterData(mss cluster.KeepersState, cv *cluster.ClusterView, prevIndex uint64) (*etcd.Response, error) {
	// write cluster view
	cd := &cluster.ClusterData{
		KeepersState: mss,
		ClusterView:  cv,
	}
	cdj, err := json.Marshal(cd)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(e.etcdPath, clusterDataFile)
	opts := &etcd.SetOptions{}
	if prevIndex == 0 {
		opts.PrevExist = etcd.PrevNoExist
	} else {
		opts.PrevExist = etcd.PrevExist
		opts.PrevIndex = prevIndex
	}
	return e.kAPI.Set(context.Background(), path, string(cdj), opts)
}

func (e *EtcdManager) GetClusterData() (*cluster.ClusterData, *etcd.Response, error) {
	var cd *cluster.ClusterData
	path := filepath.Join(e.etcdPath, clusterDataFile)
	res, err := e.kAPI.Get(context.Background(), path, &etcd.GetOptions{Quorum: true})
	if err != nil && !IsEtcdNotFound(err) {
		log.Errorf("err: %v", err)
		return nil, nil, err
	} else if !IsEtcdNotFound(err) {
		err = json.Unmarshal([]byte(res.Node.Value), &cd)
		if err != nil {
			return nil, nil, err
		}
		return cd, res, nil
	}
	return nil, nil, nil
}

func (e *EtcdManager) GetKeepersState() (cluster.KeepersState, *etcd.Response, error) {
	cd, res, err := e.GetClusterData()
	if err != nil || cd == nil {
		return nil, res, err
	}
	return cd.KeepersState, res, nil
}

func (e *EtcdManager) GetClusterView() (*cluster.ClusterView, *etcd.Response, error) {
	cd, res, err := e.GetClusterData()
	if err != nil || cd == nil {
		return nil, res, err
	}
	return cd.ClusterView, res, nil
}

func (e *EtcdManager) SetKeeperDiscoveryInfo(id string, ms *cluster.KeeperDiscoveryInfo) (*etcd.Response, error) {
	msj, err := json.Marshal(ms)
	if err != nil {
		return nil, err
	}
	return e.kAPI.Set(context.Background(), filepath.Join(e.etcdPath, discoveryInfoDir, id), string(msj), nil)
}

func (e *EtcdManager) GetKeeperDiscoveryInfo(id string) (*cluster.KeeperDiscoveryInfo, bool, error) {
	if id == "" {
		return nil, false, fmt.Errorf("empty keeper id")
	}
	var keeper cluster.KeeperDiscoveryInfo
	res, err := e.kAPI.Get(context.Background(), filepath.Join(e.etcdPath, discoveryInfoDir, id), &etcd.GetOptions{Quorum: true})
	if err != nil && !IsEtcdNotFound(err) {
		log.Errorf("err: %v", err)
		return nil, false, err
	} else if !IsEtcdNotFound(err) {
		err = json.Unmarshal([]byte(res.Node.Value), &keeper)
		if err != nil {
			return nil, false, err
		}
		return &keeper, true, nil
	}
	return nil, false, nil
}

func (e *EtcdManager) GetKeepersDiscoveryInfo() (cluster.KeepersDiscoveryInfo, error) {
	keepers := cluster.KeepersDiscoveryInfo{}
	res, err := e.kAPI.Get(context.Background(), filepath.Join(e.etcdPath, discoveryInfoDir), &etcd.GetOptions{Recursive: true, Quorum: true})
	if err != nil && !IsEtcdNotFound(err) {
		return nil, err
	} else if !IsEtcdNotFound(err) {
		for _, node := range res.Node.Nodes {
			var keeper cluster.KeeperDiscoveryInfo
			err = json.Unmarshal([]byte(node.Value), &keeper)
			if err != nil {
				return nil, err
			}
			keepers = append(keepers, &keeper)
		}
	}
	return keepers, nil
}

func (e *EtcdManager) SetProxyView(pv *cluster.ProxyView, prevIndex uint64) (*etcd.Response, error) {
	log.Debugf("prevIndex: %d", prevIndex)
	// write cluster view
	pvj, err := json.Marshal(pv)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(e.etcdPath, proxyViewFile)
	opts := &etcd.SetOptions{}
	if prevIndex == 0 {
		opts.PrevExist = etcd.PrevNoExist
	} else {
		opts.PrevExist = etcd.PrevExist
		opts.PrevIndex = prevIndex
	}
	return e.kAPI.Set(context.Background(), path, string(pvj), opts)
}

func (e *EtcdManager) DeleteProxyView(prevIndex uint64) (*etcd.Response, error) {
	path := filepath.Join(e.etcdPath, proxyViewFile)
	opts := &etcd.DeleteOptions{PrevIndex: prevIndex}
	return e.kAPI.Delete(context.Background(), path, opts)
}

func (e *EtcdManager) GetProxyView() (*cluster.ProxyView, *etcd.Response, error) {
	var pv *cluster.ProxyView
	path := filepath.Join(e.etcdPath, proxyViewFile)
	res, err := e.kAPI.Get(context.Background(), path, &etcd.GetOptions{Quorum: true})
	if err != nil && !IsEtcdNotFound(err) {
		log.Errorf("err: %v", err)
		return nil, nil, err
	} else if !IsEtcdNotFound(err) {
		err = json.Unmarshal([]byte(res.Node.Value), &pv)
		if err != nil {
			return nil, nil, err
		}
		return pv, res, nil
	}
	return nil, nil, nil
}

// IsEtcdNotFound returns true if err is an etcd not found error.
func IsEtcdNotFound(err error) bool {
	return isEtcdErrorNum(err, etcd.ErrorCodeKeyNotFound)
}

// IsEtcdNodeExist returns true if err is an etcd node aleady exist error.
func IsEtcdNodeExist(err error) bool {
	return isEtcdErrorNum(err, etcd.ErrorCodeNodeExist)
}

// IsEtcdTestFailed returns true if err is an etcd write conflict.
func IsEtcdTestFailed(err error) bool {
	return isEtcdErrorNum(err, etcd.ErrorCodeTestFailed)
}

// isEtcdErrorNum returns true if err is an etcd error, whose errorCode matches errorCode
func isEtcdErrorNum(err error, errorCode int) bool {
	etcdError, ok := err.(etcd.Error)
	return ok && etcdError.Code == errorCode
}
