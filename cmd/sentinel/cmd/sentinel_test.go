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

package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"reflect"

	"github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
)

var curUID int

var now = time.Now()

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
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.Uint16P(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   cluster.Uint16P(cluster.DefaultAdditionalWalSenders * 2),
						SynchronousReplication: cluster.BoolP(true),
						UsePgrewind:            cluster.BoolP(true),
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
						MergePgParameters:      cluster.BoolP(true),
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
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.Uint16P(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   cluster.Uint16P(cluster.DefaultAdditionalWalSenders * 2),
						SynchronousReplication: cluster.BoolP(true),
						UsePgrewind:            cluster.BoolP(true),
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
						MergePgParameters:      cluster.BoolP(true),
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
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.Uint16P(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   cluster.Uint16P(cluster.DefaultAdditionalWalSenders * 2),
						SynchronousReplication: cluster.BoolP(true),
						UsePgrewind:            cluster.BoolP(true),
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
						MergePgParameters:      cluster.BoolP(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.Uint16P(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   cluster.Uint16P(cluster.DefaultAdditionalWalSenders * 2),
						SynchronousReplication: cluster.BoolP(true),
						UsePgrewind:            cluster.BoolP(true),
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
						MergePgParameters:      cluster.BoolP(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   "keeper1",
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:                 cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders * 2,
							SynchronousReplication:      false,
							UsePgrewind:                 true,
							PGParameters:                cluster.PGParameters{"param01": "value01", "param02": "value02"},
							InitMode:                    cluster.DBInitModeNew,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							IncludeConfig:               true,
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
					},
				},
				Proxy: &cluster.Proxy{},
			},
		},
		// #2 cluster initialization, more than one keeper, the first will be choosen to be the new master.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.Uint16P(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: cluster.BoolP(true),
						UsePgrewind:            cluster.BoolP(true),
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
						MergePgParameters:      cluster.BoolP(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs:   cluster.DBs{},
				Proxy: &cluster.Proxy{},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.Uint16P(cluster.DefaultMaxStandbys * 2),
						SynchronousReplication: cluster.BoolP(true),
						MaxStandbysPerSender:   cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						UsePgrewind:            cluster.BoolP(true),
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
						InitMode:               cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
						MergePgParameters:      cluster.BoolP(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   "keeper1",
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:                 cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							UsePgrewind:                 true,
							PGParameters:                cluster.PGParameters{"param01": "value01", "param02": "value02"},
							InitMode:                    cluster.DBInitModeNew,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							IncludeConfig:               true,
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
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
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						InitMode:             cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
						MergePgParameters:    cluster.BoolP(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							InitMode:               cluster.DBInitModeNew,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
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
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						InitMode:             cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
						MergePgParameters:    cluster.BoolP(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseInitializing,
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
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
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #5 One master and one standby, master db not healthy: standby elected as new master.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db2",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #6 From the previous test, new master (db2) converged. Old master setup to follow new master (db2).
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db2",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 2,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db2",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:            "keeper2",
							RequestTimeout:       cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:          cluster.DefaultMaxStandbys,
							AdditionalWalSenders: cluster.DefaultAdditionalWalSenders,
							Role:                 common.RoleMaster,
							SynchronousReplication:      false,
							Followers:                   []string{"db3"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 2,
						},
					},
					"db3": &cluster.DB{
						UID:        "db3",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeResync,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db2",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db2",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #7 One master and one standby, master db not healthy, standby not converged (old clusterview): no standby elected as new master, clusterview not changed.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #8 One master and one standby, master healthy but not converged: standby elected as new master.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db2",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #9 One master and one standby, 3 keepers (one available). Standby ok. No new standby db on free keeper created.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(1),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper3": &cluster.Keeper{
						UID:  "keeper3",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(1),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper3": &cluster.Keeper{
						UID:  "keeper3",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #10 One master and one standby, 3 keepers (one available). Standby failed to converge (keeper healthy). New standby db on free keeper created.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(1),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper3": &cluster.Keeper{
						UID:  "keeper3",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(1),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper3": &cluster.Keeper{
						UID:  "keeper3",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2", "db3"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					"db3": &cluster.DB{
						UID:        "db3",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper3",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeResync,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: 0,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #11 From previous test. new standby db "db3" converged, old standby db removed since exceeds MaxStandbysPerSender.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(1),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper3": &cluster.Keeper{
						UID:  "keeper3",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2", "db3"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 2,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					"db3": &cluster.DB{
						UID:        "db3",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper3",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(1),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper3": &cluster.Keeper{
						UID:  "keeper3",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db3"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 2,
						},
					},
					"db3": &cluster.DB{
						UID:        "db3",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper3",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #12 One master and one standby, 2 keepers. Standby failed to converge (keeper healthy). No standby db created since there's no free keeper.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(1),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(1),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:      common.RoleStandby,
							Followers: []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #13 One master and one keeper without db assigned. keeper2 dead for more then DeadKeeperRemovalInterval: keeper2 removed.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now.Add(-100 * time.Hour),
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper1",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							Role:                        common.RoleMaster,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #14 Changed clusterSpec parameters. RequestTimeout, MaxStandbys, UsePgrewind, PGParameters should bet updated in the DBSpecs.
		{
			cd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.Uint16P(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   cluster.Uint16P(cluster.DefaultAdditionalWalSenders * 2),
						SynchronousReplication: cluster.BoolP(true),
						UsePgrewind:            cluster.BoolP(true),
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   "keeper1",
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.DBInitModeNone,
							SynchronousReplication:      false,
							UsePgrewind:                 false,
							PGParameters:                nil,
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							UsePgrewind:            false,
							PGParameters:           nil,
							Role:                   common.RoleStandby,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.ClusterData{
				Cluster: &cluster.Cluster{
					UID:        "cluster1",
					Generation: 1,
					Spec: &cluster.ClusterSpec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            cluster.Uint16P(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   cluster.Uint16P(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   cluster.Uint16P(cluster.DefaultAdditionalWalSenders * 2),
						SynchronousReplication: cluster.BoolP(true),
						UsePgrewind:            cluster.BoolP(true),
						PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: 1,
						Phase:             cluster.ClusterPhaseNormal,
						Master:            "db1",
					},
				},
				Keepers: cluster.Keepers{
					"keeper1": &cluster.Keeper{
						UID:  "keeper1",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					"keeper2": &cluster.Keeper{
						UID:  "keeper2",
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					"db1": &cluster.DB{
						UID:        "db1",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   "keeper1",
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:                 cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders * 2,
							InitMode:                    cluster.DBInitModeNone,
							SynchronousReplication:      true,
							UsePgrewind:                 true,
							PGParameters:                cluster.PGParameters{"param01": "value01", "param02": "value02"},
							Role:                        common.RoleMaster,
							Followers:                   []string{"db2"},
							SynchronousStandbys:         []string{"db2"},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
					"db2": &cluster.DB{
						UID:        "db2",
						Generation: 2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              "keeper2",
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders * 2,
							InitMode:               cluster.DBInitModeNone,
							SynchronousReplication: false,
							UsePgrewind:            true,
							PGParameters:           cluster.PGParameters{"param01": "value01", "param02": "value02"},
							Role:                   common.RoleStandby,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: "db1",
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: 1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "db1",
						EnabledProxies: []string{},
					},
				},
			},
		},
	}

	for i, tt := range tests {
		s := &Sentinel{uid: "sentinel01", UIDFn: testUIDFn, RandFn: testRandFn, dbConvergenceInfos: make(map[string]*DBConvergenceInfo)}

		// reset curUID func value to latest db uid
		curUID = 0
		for _, db := range tt.cd.DBs {
			uid, _ := strconv.Atoi(strings.TrimPrefix(db.UID, "db"))
			if uid > curUID {
				curUID = uid
			}
		}

		// Populate db convergence timers, these are populated with a negative timer to make them result like not converged.
		for _, db := range tt.cd.DBs {
			s.dbConvergenceInfos[db.UID] = &DBConvergenceInfo{Generation: 0, Timer: int64(-1000 * time.Hour)}
		}

		fmt.Printf("test #%d\n", i)
		t.Logf("test #%d", i)

		outcd, err := s.updateCluster(tt.cd, cluster.ProxiesInfo{})
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
	curUID++
	return fmt.Sprintf("%s%d", "db", curUID)
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
