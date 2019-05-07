// Copyright 2019 Sorint.lab
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

package register

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/mock/store"
)

func TestNewCluster(t *testing.T) {
	t.Run("should return error returned by store when getting cluster data", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStore := mock_store.NewMockStore(ctrl)
		mockStore.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, errors.New("unable to fetch cluster data"))

		_, err := NewCluster("test", Config{}, mockStore)

		if err == nil || err.Error() != "unable to fetch cluster data" {
			t.Errorf("expected unable to fetch cluster data error")
		}
	})

	t.Run("should return error returned by if store returns nil as cluster data", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStore := mock_store.NewMockStore(ctrl)
		mockStore.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, nil)

		_, err := NewCluster("test", Config{}, mockStore)

		if err == nil || err.Error() != "no cluster data available" {
			t.Errorf("expected no cluster data available error")
		}
	})

	t.Run("should create new cluster", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cd := &cluster.ClusterData{}

		mockStore := mock_store.NewMockStore(ctrl)
		mockStore.EXPECT().GetClusterData(gomock.Any()).Return(cd, nil, nil)

		expected := Cluster{name: "test", cd: cd}

		actual, err := NewCluster("test", Config{}, mockStore)

		if expected.name != actual.name {
			t.Errorf("expected name to be %s but got %s", expected.name, actual.name)
		} else if expected.cd != actual.cd {
			t.Errorf("expected cluster data to be %v but got %v", expected.cd, actual.cd)
		}

		if err != nil {
			t.Errorf("haven't expected error when fetching cluster data")
		}
	})
}

func TestServiceInfos(t *testing.T) {
	t.Run("should return error if cluster data not available", func(t *testing.T) {
		cl := Cluster{cd: &cluster.ClusterData{}, name: "test"}

		_, err := cl.ServiceInfos()

		if err == nil || err.Error() != "cluster data not available" {
			t.Errorf("expected cluster data not available error")
		}
	})

	t.Run("should get all the healthy service infos form the cluster data", func(t *testing.T) {
		master := &cluster.DB{UID: "master", Status: cluster.DBStatus{Healthy: true, ListenAddress: "127.0.0.1", Port: "5432"}}
		slave := &cluster.DB{UID: "slave1", Status: cluster.DBStatus{Healthy: true, ListenAddress: "127.0.0.1", Port: "5433"}}
		anotherSlave := &cluster.DB{UID: "slave2", Status: cluster.DBStatus{Healthy: false, ListenAddress: "127.0.0.1", Port: "5433"}}
		cl := Cluster{
			cd: &cluster.ClusterData{
				DBs: map[string]*cluster.DB{"master": master, "slave1": slave, "slave2": anotherSlave},
				Cluster: &cluster.Cluster{
					Status: cluster.ClusterStatus{Master: "master"},
				},
			},
			name:        "test",
			tagMasterAs: Tags{"master"},
			tagSlaveAs:  Tags{"slave"},
		}

		infos, err := cl.ServiceInfos()

		if err != nil {
			t.Error("expected no error")
		}

		if len(infos) != 2 {
			t.Errorf("expected to have 2 service infos but got %d", len(infos))
		}
		expectedMasterInfo := ServiceInfo{
			Name:     "test",
			Address:  "127.0.0.1",
			Port:     5432,
			ID:       "master",
			Tags:     Tags{"master"},
			IsMaster: true,
		}

		expectedSlaveInfo := ServiceInfo{
			Name:     "test",
			Address:  "127.0.0.1",
			Port:     5433,
			ID:       "slave1",
			Tags:     Tags{"slave"},
			IsMaster: false,
		}

		actualMaster := infos["master"]
		actualSlave := infos["slave1"]

		if !expectedMasterInfo.Compare(actualMaster) {
			t.Errorf("expected master to be %v but was %v", expectedMasterInfo, actualMaster)
		}

		if !expectedMasterInfo.IsMaster {
			t.Errorf("expected isMaster to be %v but was %v", true, expectedMasterInfo.IsMaster)
		}

		if !expectedSlaveInfo.Compare(actualSlave) {
			t.Errorf("expected slave to be %v but was %v", expectedSlaveInfo, actualSlave)
		}

		if expectedSlaveInfo.IsMaster {
			t.Errorf("expected isMaster to be %v but was %v", false, expectedMasterInfo.IsMaster)
		}
	})
}
