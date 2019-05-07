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

package cmd

import (
	"github.com/golang/mock/gomock"
	"github.com/sorintlab/stolon/cmd/stolonctl/cmd/internal/mock/register"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/mock/store"
	"github.com/sorintlab/stolon/internal/store"
	"testing"

	"github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/cmd/stolonctl/cmd/register"
)

func TestCheckConfig(t *testing.T) {

	t.Run("should check for cluster name", func(t *testing.T) {
		c := config{}
		rc := register.Config{}
		err := checkConfig(&c, &rc)

		if err == nil || err.Error() != "cluster name required" {
			t.Errorf("expected cluster name required error")
		}
	})

	t.Run("should check for store backend", func(t *testing.T) {
		c := config{CommonConfig: cmd.CommonConfig{ClusterName: "test"}}
		rc := register.Config{}
		err := checkConfig(&c, &rc)

		if err == nil || err.Error() != "store backend type required" {
			t.Errorf("expected store backend type required error")
		}
	})

	t.Run("should check for consul register backend", func(t *testing.T) {
		c := config{CommonConfig: cmd.CommonConfig{ClusterName: "test", StoreBackend: "consul"}}
		rc := register.Config{Backend: "something other than consul"}
		err := checkConfig(&c, &rc)

		if err == nil || err.Error() != "unknown register backend: \"something other than consul\"" {
			t.Errorf("expected unknown register backend but got %s", err.Error())
		}
	})

	t.Run("should not return any error if all valid configurations are specified", func(t *testing.T) {
		c := config{CommonConfig: cmd.CommonConfig{ClusterName: "test", StoreBackend: "consul"}}
		rc := register.Config{Backend: "consul"}
		err := checkConfig(&c, &rc)

		if err != nil {
			t.Errorf("expected no error but got '%v'", err.Error())
		}
	})
}

func TestCheckAndRegisterMasterAndSlaves(t *testing.T) {
	t.Run("should deregister all the discovered services", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStore := mock_store.NewMockStore(ctrl)
		mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

		clusterName := "test-cluster"
		serviceInfo := register.ServiceInfo{Name: clusterName, ID: "uid1", Tags: []string{"slave"}}
		anotherServiceInfo := register.ServiceInfo{Name: clusterName, ID: "uid2", Tags: []string{"slave"}}
		discoveredServices := register.ServiceInfos{
			"uid1": serviceInfo,
			"uid2": anotherServiceInfo,
		}

		mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
		clusterData := cluster.ClusterData{
			Cluster: &cluster.Cluster{},
			DBs:     cluster.DBs{},
		}
		mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)
		mockServiceDiscovery.EXPECT().DeRegister(&anotherServiceInfo)
		mockServiceDiscovery.EXPECT().DeRegister(&serviceInfo)

		checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, false)
	})

	t.Run("should register all the existing services", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStore := mock_store.NewMockStore(ctrl)
		mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

		clusterName := "test-cluster"
		serviceInfo := register.ServiceInfo{Name: clusterName, ID: "uid1", Port: 5432, Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5432", Interval: "10s"}}
		anotherServiceInfo := register.ServiceInfo{Name: clusterName, Port: 5433, ID: "uid2", Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5433", Interval: "10s"}}
		discoveredServices := register.ServiceInfos{}

		mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
		clusterData := cluster.ClusterData{
			Cluster: &cluster.Cluster{},
			DBs: cluster.DBs{
				"uid1": &cluster.DB{
					UID:    "uid1",
					Status: cluster.DBStatus{Port: "5432", Healthy: true},
				},
				"uid2": &cluster.DB{
					UID:    "uid2",
					Status: cluster.DBStatus{Port: "5433", Healthy: true},
				},
			},
		}
		mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)
		mockServiceDiscovery.EXPECT().Register(&serviceInfo)
		mockServiceDiscovery.EXPECT().Register(&anotherServiceInfo)

		checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, false)
	})

	t.Run("should register existing services and deregister the discovered service which are no longer available", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStore := mock_store.NewMockStore(ctrl)
		mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

		clusterName := "test-cluster"
		serviceInfo := register.ServiceInfo{Name: clusterName, ID: "uid1", Port: 5432, Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5432", Interval: "10s"}}
		anotherServiceInfo := register.ServiceInfo{Name: clusterName, Port: 5433, ID: "uid2", Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5433", Interval: "10s"}}
		yetAnotherServiceInfo := register.ServiceInfo{Name: clusterName, Port: 5434, ID: "uid3", Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5433", Interval: "10s"}}
		discoveredServices := register.ServiceInfos{"uid1": serviceInfo, "uid2": anotherServiceInfo, "uid3": yetAnotherServiceInfo}

		mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
		clusterData := cluster.ClusterData{
			Cluster: &cluster.Cluster{},
			DBs: cluster.DBs{
				"uid1": &cluster.DB{
					UID:    "uid1",
					Status: cluster.DBStatus{Port: "5432", Healthy: true},
				},
				"uid2": &cluster.DB{
					UID:    "uid2",
					Status: cluster.DBStatus{Port: "5433", Healthy: true},
				},
			},
		}
		mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)
		mockServiceDiscovery.EXPECT().DeRegister(&yetAnotherServiceInfo)

		checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, false)
	})

	t.Run("master registration is not allowed", func(t *testing.T) {
		t.Run("should deregister the master even it exists", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mock_store.NewMockStore(ctrl)
			mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

			clusterName := "test-cluster"
			serviceInfo := register.ServiceInfo{Name: clusterName, ID: "uid1", Port: 5432, Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5432", Interval: "10s"}}
			masterServiceInfo := register.ServiceInfo{Name: clusterName, Port: 5433, ID: "uid2", Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5433", Interval: "10s"}, IsMaster: true}
			discoveredServices := register.ServiceInfos{"uid1": serviceInfo, "uid2": masterServiceInfo}

			mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
			clusterData := cluster.ClusterData{
				Cluster: &cluster.Cluster{Status: cluster.ClusterStatus{Master: "uid2"}},
				DBs: cluster.DBs{
					"uid1": &cluster.DB{
						UID:    "uid1",
						Status: cluster.DBStatus{Port: "5432", Healthy: true},
					},
					"uid2": &cluster.DB{
						UID:    "uid2",
						Status: cluster.DBStatus{Port: "5433", Healthy: true},
					},
				},
			}
			mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)
			mockServiceDiscovery.EXPECT().DeRegister(&masterServiceInfo)

			checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, false)
		})

		t.Run("should deregister if discovered service is master", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mock_store.NewMockStore(ctrl)
			mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

			clusterName := "test-cluster"
			masterService := register.ServiceInfo{IsMaster: true, Name: clusterName, Port: 5434, ID: "uid2", Tags: []string{"master"}, Check: register.HealthCheck{TCP: ":5433", Interval: "10s"}}
			discoveredServices := register.ServiceInfos{"uid3": masterService}

			mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
			clusterData := cluster.ClusterData{
				Cluster: &cluster.Cluster{Status: cluster.ClusterStatus{Master: "uid2"}},
				DBs: cluster.DBs{
					"uid2": &cluster.DB{
						UID:    "uid2",
						Status: cluster.DBStatus{Port: "5433", Healthy: true},
					},
				},
			}
			mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)
			mockServiceDiscovery.EXPECT().DeRegister(&masterService)

			checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, false)
		})

		t.Run("should not register if existing service is master", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mock_store.NewMockStore(ctrl)
			mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

			clusterName := "test-cluster"
			discoveredServices := register.ServiceInfos{}

			mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
			clusterData := cluster.ClusterData{
				Cluster: &cluster.Cluster{Status: cluster.ClusterStatus{Master: "uid1"}},
				DBs: cluster.DBs{
					"uid1": &cluster.DB{
						UID:    "uid1",
						Status: cluster.DBStatus{Port: "5432", Healthy: true},
					},
				},
			}
			mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)

			checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, false)
		})
	})

	t.Run("master registration is allowed", func(t *testing.T) {
		t.Run("should register the master if its discovered", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mock_store.NewMockStore(ctrl)
			mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

			clusterName := "test-cluster"
			serviceInfo := register.ServiceInfo{Name: clusterName, ID: "uid1", Port: 5432, Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5432", Interval: "10s"}}
			masterServiceInfo := register.ServiceInfo{Name: clusterName, Port: 5433, ID: "uid2", Tags: []string{"master"}, Check: register.HealthCheck{TCP: ":5433", Interval: "10s"}, IsMaster: true}
			discoveredServices := register.ServiceInfos{"uid1": serviceInfo}

			mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
			clusterData := cluster.ClusterData{
				Cluster: &cluster.Cluster{Status: cluster.ClusterStatus{Master: "uid2"}},
				DBs: cluster.DBs{
					"uid1": &cluster.DB{
						UID:    "uid1",
						Status: cluster.DBStatus{Port: "5432", Healthy: true},
					},
					"uid2": &cluster.DB{
						UID:    "uid2",
						Status: cluster.DBStatus{Port: "5433", Healthy: true},
					},
				},
			}
			mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)
			mockServiceDiscovery.EXPECT().Register(&masterServiceInfo)

			checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, true)
		})

		t.Run("should deregister if non existing is master", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mock_store.NewMockStore(ctrl)
			mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

			clusterName := "test-cluster"
			masterService := register.ServiceInfo{IsMaster: true, Name: clusterName, Port: 5434, ID: "uid3", Tags: []string{"slave"}, Check: register.HealthCheck{TCP: ":5433", Interval: "10s"}}
			discoveredServices := register.ServiceInfos{"uid3": masterService}

			mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
			clusterData := cluster.ClusterData{
				Cluster: &cluster.Cluster{},
				DBs:     cluster.DBs{},
			}
			mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)
			mockServiceDiscovery.EXPECT().DeRegister(&masterService)

			checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, true)
		})

		t.Run("should register if existing service is master", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mock_store.NewMockStore(ctrl)
			mockServiceDiscovery := mock_register.NewMockServiceDiscovery(ctrl)

			clusterName := "test-cluster"
			discoveredServices := register.ServiceInfos{}
			masterService := register.ServiceInfo{IsMaster: true, Name: clusterName, Port: 5432, ID: "uid1", Tags: []string{"master"}, Check: register.HealthCheck{TCP: ":5432", Interval: "10s"}}

			mockServiceDiscovery.EXPECT().Services(clusterName).Return(discoveredServices, nil)
			clusterData := cluster.ClusterData{
				Cluster: &cluster.Cluster{Status: cluster.ClusterStatus{Master: "uid1"}},
				DBs: cluster.DBs{
					"uid1": &cluster.DB{
						UID:    "uid1",
						Status: cluster.DBStatus{Port: "5432", Healthy: true},
					},
				},
			}
			mockStore.EXPECT().GetClusterData(gomock.Any()).Return(&clusterData, &store.KVPair{}, nil)
			mockServiceDiscovery.EXPECT().Register(&masterService)

			checkAndRegisterMasterAndSlaves(clusterName, mockStore, mockServiceDiscovery, true)
		})
	})
}
