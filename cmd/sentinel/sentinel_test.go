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

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"reflect"
)

var valTrue = true

func TestUpdateCluster(t *testing.T) {
	tests := []struct {
		cd    *cluster.ClusterData
		outcd *cluster.ClusterData
		err   error
	}{
		// Init phase, also test dbSpec paramaters copied from clusterSpec.
		// #0 cluster initialization, no keepers
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            cluster.Duration{Duration: cluster.DefaultInitTimeout},
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeNew,
						MergePgParameters:      &valTrue,
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
					},
				},
				Keepers: cluster.Keepers{},
				DBs:     cluster.DBs{},
				Proxy:   &cluster.Proxy{},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            cluster.Duration{Duration: cluster.DefaultInitTimeout},
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeNew,
						MergePgParameters:      &valTrue,
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
					},
				},
				Keepers: cluster.Keepers{},
				DBs:     cluster.DBs{},
				Proxy:   &cluster.Proxy{},
			},
			err: fmt.Errorf("cannot choose initial master: no keepers registered"),
		},
		// #1 cluster initialization, one keeper
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            cluster.Duration{Duration: cluster.DefaultInitTimeout},
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeNew,
						MergePgParameters:      &valTrue,
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            cluster.Duration{Duration: cluster.DefaultInitTimeout},
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeNew,
						MergePgParameters:      &valTrue,
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper01",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							SynchronousReplication: true,
							UsePgrewind:            true,
							PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
							InitMode:               cluster.DBInitModeNew,
							Role:                   common.RoleMaster,
							Followers:              []string{},
							IncludeConfig:          true,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		{
			// #2 cluster initialization, more than one keeper, the first will be choosen to be the new master.
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            cluster.Duration{Duration: cluster.DefaultInitTimeout},
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeNew,
						MergePgParameters:      &valTrue,
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            cluster.Duration{Duration: cluster.DefaultInitTimeout},
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeNew,
						MergePgParameters:      &valTrue,
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper01",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							SynchronousReplication: true,
							UsePgrewind:            true,
							PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
							InitMode:               cluster.DBInitModeNew,
							Role:                   common.RoleMaster,
							Followers:              []string{},
							IncludeConfig:          true,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		// #3 cluster initialization, keeper initialization failed
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
						InitMode:           cluster.ClusterInitModeNew,
						MergePgParameters:  &valTrue,
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							InitMode:  cluster.DBInitModeNew,
							Role:      common.RoleMaster,
							Followers: []string{},
						},
						Status: cluster.DBStatus{
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
						InitMode:           cluster.ClusterInitModeNew,
						MergePgParameters:  &valTrue,
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
		},

		// Normal phase
		// #4 One master and one standby, both healthy: no change from previous cd
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleMaster,
							Followers: []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db01",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db01",
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleMaster,
							Followers: []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db01",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db01",
					},
				},
			},
		},
		// #5 One master and one standby, master db not healthy: standby elected as new master.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleMaster,
							Followers: []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db01",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db01",
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db02",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleUndefined,
							Followers: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleMaster,
							Followers: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		// #6 From the previous test, new master (db02) converged. Old master setup to follow new master (db02).
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db02",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleMaster,
							Followers: []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleMaster,
							Followers: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 2,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db02",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db02",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleMaster,
							Followers: []string{"db01"},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 2,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db02",
					},
				},
			},
		},
		// #7 One master and one standby, master db not healthy, standby not converged (old clusterview): no standby elected as new master, clusterview not changed.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleMaster,
							Followers: []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db01",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db01",
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleMaster,
							Followers: []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db01",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db01",
					},
				},
			},
		},
		// #8 One master and one standby, master healthy but not converged: standby elected as new master.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleMaster,
							Followers: []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db01",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db01",
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout: cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:        cluster.Duration{Duration: cluster.DefaultInitTimeout},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db02",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper01",
							Role:      common.RoleUndefined,
							Followers: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID: "keeper02",
							Role:      common.RoleMaster,
							Followers: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		// #9 Changed clusterSpec parameters. RequestTimeout, MaxStandbys, SynchronousReplication, UsePgrewind, PGParameters should bet updated in the DBSpecs.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            cluster.Duration{Duration: cluster.DefaultInitTimeout},
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper01",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							SynchronousReplication: false,
							UsePgrewind:            false,
							PGParameters:           nil,
							Role:                   common.RoleMaster,
							Followers:              []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper02",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							SynchronousReplication: false,
							UsePgrewind:            false,
							PGParameters:           nil,
							Role:                   common.RoleStandby,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db01",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db01",
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster01",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            cluster.Duration{Duration: cluster.DefaultInitTimeout},
						RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.DefaultMaxStandbys * 2,
						SynchronousReplication: true,
						UsePgrewind:            true,
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db01",
					},
				},
				Keepers: cluster.Keepers{
					"keeper01": &cluster.Keeper{
						UID:  "keeper01",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
					"keeper02": &cluster.Keeper{
						UID:  "keeper02",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy: true,
						},
					},
				},
				DBs: cluster.DBs{
					"db01": &cluster.DB{
						UID:        "db01",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper01",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							SynchronousReplication: true,
							UsePgrewind:            true,
							PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
							Role:                   common.RoleMaster,
							Followers:              []string{"db02"},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db02": &cluster.DB{
						UID:        "db02",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper02",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							SynchronousReplication: true,
							UsePgrewind:            true,
							PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
							Role:                   common.RoleStandby,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db01",
							},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Spec: cluster.ProxySpec{
						MasterDBUID: "db01",
					},
				},
			},
		},
	}

	for i, tt := range tests {
		s := &Sentinel{id: "id", UIDFn: testUIDFn, RandFn: testRandFn}

		outcd, err := s.updateCluster(tt.cd)
		t.Logf("test #%d", i)
		if tt.err != nil {
			if err == nil {
				t.Errorf("got no error, wanted error: %v", tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("got error: %v, wanted error: %v", err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if !testEqualCD(outcd, tt.outcd) {
				t.Errorf("wrong outcd: got:\n%s\nwant:\n%s", spew.Sdump(outcd), spew.Sdump(tt.outcd))
			}
		}
	}
}

func testUIDFn() string {
	return "db01"
}

func testRandFn(i int) int {
	return 0
}

func testEqualCD(cd1, cd2 *cluster.ClusterData) bool {
	// ignore times
	for _, cd := range []*cluster.ClusterData{cd1, cd2} {
		cd.Cluster.ChangeTime = time.Time{}
		for _, k := range cd.Keepers {
			k.ChangeTime = time.Time{}
		}
		for _, db := range cd.DBs {
			db.ChangeTime = time.Time{}
		}
		cd.Proxy.ChangeTime = time.Time{}
	}
	return reflect.DeepEqual(cd1, cd2)

}
