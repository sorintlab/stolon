// Copyright 2026 PgVillage
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
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	cluster "github.com/pgvillage-tools/stolon/api/v1"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/timer"
	"github.com/pgvillage-tools/stolon/internal/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
)

var curUID int

var newCluster = cluster.New

var now = time.Now()

func TestUpdateCluster(t *testing.T) {
	const (
		p1 = "param01"
		v1 = "value01"
		p2 = "param02"
		v2 = "value02"

		k1 = "keeper1"
		k2 = "keeper2"
		k3 = "keeper3"

		c1 = "cluster1"

		d1 = "db1"
		d2 = "db2"
		d3 = "db3"

		g1 = 1
		g2 = 2
		g3 = 3
		g4 = 4
	)
	ctx := context.Background()
	tests := []struct {
		cd    *cluster.Data
		outcd *cluster.Data
		err   error
	}{
		// Init phase, also test dbSpec parameters copied from clusterSpec.
		// #0 cluster initialization, no keepers
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{},
				DBs:     cluster.DBs{},
				Proxy:   &cluster.Proxy{},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{},
				DBs:     cluster.DBs{},
				Proxy:   &cluster.Proxy{},
			},
			err: errors.New("cannot choose initial master: no keepers registered"),
		},
		// #1 cluster initialization, one keeper
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
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
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders * 2,
							SynchronousReplication: false,
							UsePgrewind:            true,
							// revive:disable-next-line
							PGParameters:                cluster.PGParameters{p1: v1, p2: v2},
							InitMode:                    cluster.NewDB,
							Role:                        common.RolePrimary,
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
		// #2 cluster initialization, more than one keeper, the first will be chosen to be the new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
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
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						SynchronousReplication: util.ToPtr(true),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
						InitMode:               &newCluster,
						MergePgParameters:      util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							UsePgrewind:            true,
							PGParameters: cluster.PGParameters{
								p1: v1,
								p2: v2,
							},
							InitMode:                    cluster.NewDB,
							Role:                        common.RolePrimary,
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
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						InitMode:             &newCluster,
						MergePgParameters:    util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							InitMode:                    cluster.NewDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
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
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						InitMode:             &newCluster,
						MergePgParameters:    util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Initializing,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
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
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #5 One master and one standby, master db not healthy: standby elected as new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #6 From the previous test, new master (db2) converged. Old master setup to follow new master (db2).
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g2,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d2: &cluster.DB{
						UID:        d2,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							Role:                        common.RolePrimary,
							SynchronousReplication:      false,
							Followers:                   []string{d3},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g2,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.ResyncDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d2,
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
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d2,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #7 One master and one standby, master db not healthy, standby not converged
		// (old clusterview): no standby elected as new master, clusterview not changed.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
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
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
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
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #8 One master and one standby, master healthy but not converged: standby elected as new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #9 One master and one standby, 3 keepers (one available).
		// Standby ok.
		// No new standby db on free keeper created.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #10 One master and one standby, 3 keepers (one available).
		// Standby failed to converge (keeper healthy).
		// New standby db on free keeper created.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
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
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.ResyncDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
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
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #11 From previous test.
		// new standby db d3 converged, old standby db removed since exceeds MaxStandbysPerSender.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g2,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: 0,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d3},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g2,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #12 One master and one standby, 2 keepers. Standby failed to converge (keeper healthy).
		// No standby db created since there's no free keeper.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(uint16(1)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #13 One master and one keeper without db assigned.
		// keeper2 dead for more then DeadKeeperRemovalInterval: keeper2 removed.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now.Add(-100 * time.Hour),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:   &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:          &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:          &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender: util.ToPtr(cluster.DefaultMaxStandbysPerSender),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #14 Changed clusterSpec parameters.
		// RequestTimeout, MaxStandbys, UsePgrewind, PGParameters should bet updated in the DBSpecs.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      false,
							UsePgrewind:                 false,
							PGParameters:                nil,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							UsePgrewind:            false,
							PGParameters:           nil,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						RequestTimeout:         &cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
						MaxStandbys:            util.ToPtr(cluster.DefaultMaxStandbys * 2),
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						AdditionalWalSenders:   util.ToPtr(uint16(cluster.DefaultAdditionalWalSenders) * 2),
						SynchronousReplication: util.ToPtr(true),
						UsePgrewind:            util.ToPtr(true),
						PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k1,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders * 2,
							InitMode:               cluster.NoDB,
							SynchronousReplication: true,
							UsePgrewind:            true,
							PGParameters: cluster.PGParameters{
								p1: v1,
								p2: v2,
							},
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout * 2},
							MaxStandbys:            cluster.DefaultMaxStandbys * 2,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders * 2,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							UsePgrewind:            true,
							PGParameters:           cluster.PGParameters{p1: v1, p2: v2},
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #15 One master and one standby all healthy. Synchronous replication
		// enabled right now in the cluster spec.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{fakeStandbyName},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #16 One master and one standby. Synchronous replication enabled right
		// now in the cluster spec.
		// master db not healthy: standby elected as new master since
		// dbSpec.SynchronousReplication is false yet. The new master will have
		// SynchronousReplication true and a fake sync stanby.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{fakeStandbyName},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #17 One master and one standby. Synchronous replication already
		// enabled.
		// master db not healthy: standby elected as new master since it's in
		// the SynchronousStandbys.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d1},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #18 One master and one standby. Synchronous replication already
		// enabled.
		// stanby db not healthy: standby kept inside synchronousStandbys since
		// there's not better standby to choose
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #19 One master and two standbys. Synchronous replication already
		// enabled with MinSynchronousStandbys and MaxSynchronousStandbys to 1
		// (default).
		// sync standby db2 not healthy: the other standby db3 choosed as new
		// sync standby. Both will appear as SynchronousStandbys
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #20 From previous. master haven't yet reported the new sync standbys (db2, db3).
		// master db is not healty. db2 is the unique in common between the
		// reported (db2) and the required in the spec (db2, db3) but it's not
		// healty so no master could be elected.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #21 From #19. master have not yet reported the new sync standbys as in sync (db3).
		// db2 will remain the unique real in sync db in db1.Status.SynchronousStandbys
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #22 From #21. master have not yet reported the new sync standbys as in sync (db3) and db3 is failed.
		// db2 will remain the unique real in sync db in
		// db1.Status.SynchronousStandbys and also db1.Spec.SynchronousStandbys
		// will contain only db2 (db3 removed)
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #23 From #19. master have reported the new sync standbys as in sync (db2, db3).
		// db2 will be removed from synchronousStandbys
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d2, d3},
							CurSynchronousStandbys: []string{d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      2,
							SynchronousStandbys:    []string{d3},
							CurSynchronousStandbys: []string{d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #24 From previous.
		// master db is not healty. db3 is the unique in common between the
		// reported (db2, db3) and the required in the spec (db3) so it'll be
		// elected as new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g3,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   2,
							SynchronousStandbys: []string{d2, d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d3,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g4,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   2,
							SynchronousStandbys: []string{d2, d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k3,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d1},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #25 One master and two standbys. Synchronous replication already
		// enabled with MinSynchronousStandbys and MaxSynchronousStandbys to 2.
		// master (db1) and db2 failed, db3 elected as master.
		// This test checks that the db3 synchronousStandbys are correctly sorted
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2, d3},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2, d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k3,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d3,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k3: &cluster.Keeper{
						UID:  k3,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d2, d3},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2, d3},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d3: &cluster.DB{
						UID:        d3,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k3,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d1, d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #26 One master (unhealthy) and an async standby. Synchronous replication already enabled
		// with MinSynchronousStandbys and MaxSynchronousStandbys to 1 (default)
		// master (db1) and async (db2) with --never-synchronous-replica.
		// db2 is never elected as new sync.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:                 true,
							LastHealthyTime:         now,
							CanBeSynchronousReplica: util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{"stolonfakestandby"},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:                 true,
							LastHealthyTime:         now,
							CanBeSynchronousReplica: util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{"stolonfakestandby"},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #27 One master (unhealthy) and a sync standby. Synchronous replication already
		// enabled with MinSynchronousStandbys and MaxSynchronousStandbys to 1 (default)
		// master (db1) and sync (db2) with --never-master.
		// db2 is never promoted as new master.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
							CanBeMaster:     util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                false,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
							CanBeMaster:     util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                false,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{d2},
							CurSynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #28 One master and one standby. Synchronous replication enabled right
		// now in the cluster spec, with MinSynchronousStandbys=0.
		// master db not healthy: standby elected as new master since
		// dbSpec.SynchronousReplication is false yet. The new master will have
		// SynchronousReplication true and NO fake sync standby.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      false,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #29 One master and one standby. Synchronous replication already
		// enabled, with MinSynchronousStandbys=0.
		// master db not healthy: standby elected as new master since it's in
		// the SynchronousStandbys. No fake replica is added.
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d2,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             false,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k2,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{},
							SynchronousStandbys:         []string{d1},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g2,
					Spec: cluster.ProxySpec{
						MasterDBUID:    "",
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #30 One master and one standby. Synchronous replication already
		// enabled, with MinSynchronousStandbys=0.
		// standby db not healthy: standby removed from synchronousStandbys even though
		// there's not better standby to choose
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{d2},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{d2},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:             true,
							CurrentGeneration:   1,
							SynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           false,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
		// #31 One master (unhealthy) and an async standby. Synchronous replication already enabled
		// enabled, with MinSynchronousStandbys=0.
		// master (db1) and async (db2) with --never-synchronous-replica.
		// StrictSyncRepl is set to false. Db2 is never elected as new sync, and fake replica
		// is removed from db1
		{
			cd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:                 true,
							LastHealthyTime:         now,
							CanBeSynchronousReplica: util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{"stolonfakestandby"},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
			outcd: &cluster.Data{
				Cluster: &cluster.Cluster{
					UID:        c1,
					Generation: g1,
					Spec: &cluster.Spec{
						ConvergenceTimeout:     &cluster.Duration{Duration: cluster.DefaultConvergenceTimeout},
						InitTimeout:            &cluster.Duration{Duration: cluster.DefaultInitTimeout},
						SyncTimeout:            &cluster.Duration{Duration: cluster.DefaultSyncTimeout},
						MaxStandbysPerSender:   util.ToPtr(cluster.DefaultMaxStandbysPerSender),
						SynchronousReplication: util.ToPtr(true),
						MinSynchronousStandbys: util.ToPtr(uint16(0)),
					},
					Status: cluster.ClusterStatus{
						CurrentGeneration: g1,
						Phase:             cluster.Normal,
						Master:            d1,
					},
				},
				Keepers: cluster.Keepers{
					k1: &cluster.Keeper{
						UID:  k1,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:         true,
							LastHealthyTime: now,
						},
					},
					k2: &cluster.Keeper{
						UID:  k2,
						Spec: &cluster.KeeperSpec{},
						Status: cluster.KeeperStatus{
							Healthy:                 true,
							LastHealthyTime:         now,
							CanBeSynchronousReplica: util.ToPtr(false),
						},
					},
				},
				DBs: cluster.DBs{
					d1: &cluster.DB{
						UID:        d1,
						Generation: g2,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:                   k1,
							RequestTimeout:              cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:                 cluster.DefaultMaxStandbys,
							AdditionalWalSenders:        cluster.DefaultAdditionalWalSenders,
							InitMode:                    cluster.NoDB,
							SynchronousReplication:      true,
							Role:                        common.RolePrimary,
							Followers:                   []string{d2},
							SynchronousStandbys:         []string{},
							ExternalSynchronousStandbys: []string{},
						},
						Status: cluster.DBStatus{
							Healthy:                true,
							CurrentGeneration:      1,
							SynchronousStandbys:    []string{},
							CurSynchronousStandbys: []string{},
						},
					},
					d2: &cluster.DB{
						UID:        d2,
						Generation: g1,
						ChangeTime: time.Time{},
						Spec: &cluster.DBSpec{
							KeeperUID:              k2,
							RequestTimeout:         cluster.Duration{Duration: cluster.DefaultRequestTimeout},
							MaxStandbys:            cluster.DefaultMaxStandbys,
							AdditionalWalSenders:   cluster.DefaultAdditionalWalSenders,
							InitMode:               cluster.NoDB,
							SynchronousReplication: false,
							Role:                   common.RoleReplica,
							Followers:              []string{},
							FollowConfig: &cluster.FollowConfig{
								Type:  cluster.FollowTypeInternal,
								DBUID: d1,
							},
							SynchronousStandbys:         nil,
							ExternalSynchronousStandbys: nil,
						},
						Status: cluster.DBStatus{
							Healthy:           true,
							CurrentGeneration: g1,
						},
					},
				},
				Proxy: &cluster.Proxy{
					Generation: g1,
					Spec: cluster.ProxySpec{
						MasterDBUID:    d1,
						EnabledProxies: []string{},
					},
				},
			},
		},
	}

	for i, tt := range tests {
		s := &Sentinel{uid: "sentinel01", UIDFn: testUIDFn, RandFn: testRandFn,
			dbConvergenceInfos: map[string]*DBConvergenceInfo{}}

		// reset curUID func value to latest db uid
		curUID = 0
		for _, db := range tt.cd.DBs {
			uid, _ := strconv.Atoi(strings.TrimPrefix(db.UID, "db"))
			if uid > curUID {
				curUID = uid
			}
		}

		// Populate db convergence timers, these are populated with a negative
		// timer to make them result like not converged.
		for _, db := range tt.cd.DBs {
			s.dbConvergenceInfos[db.UID] = &DBConvergenceInfo{Generation: 0, Timer: int64(-1000 * time.Hour)}
		}

		fmt.Printf("test #%d\n", i)
		t.Logf("test #%d", i)

		outcd, err := s.updateCluster(ctx, tt.cd, cluster.ProxiesInfo{})
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
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.outcd, outcd))
			}
		}
	}
}

func TestActiveProxiesInfos(t *testing.T) {
	const (
		pr1 = "proxy01"
		pr2 = "proxy02"
	)
	proxyInfo1 := cluster.ProxyInfo{UID: pr1, InfoUID: "infoUID1", ProxyTimeout: cluster.DefaultProxyTimeout}
	proxyInfo2 := cluster.ProxyInfo{UID: pr2, InfoUID: "infoUID2", ProxyTimeout: cluster.DefaultProxyTimeout}
	proxyInfoWithDifferentInfoUID := cluster.ProxyInfo{UID: pr2, InfoUID: "differentInfoUID"}
	var (
		secToNanoSecondMultiplier int64 = 1000000000

		td15ns = 15 * secToNanoSecondMultiplier
		td45ns = 45 * secToNanoSecondMultiplier
	)
	tests := []struct {
		name                       string
		proxyInfoHistories         ProxyInfoHistories
		proxiesInfos               cluster.ProxiesInfo
		expectedActiveProxies      cluster.ProxiesInfo
		expectedProxyInfoHistories ProxyInfoHistories
	}{
		{
			name:                       "should do nothing when called with empty proxyInfos",
			proxyInfoHistories:         nil,
			proxiesInfos:               cluster.ProxiesInfo{},
			expectedActiveProxies:      cluster.ProxiesInfo{},
			expectedProxyInfoHistories: nil,
		},
		{
			name:                  "should append to histories when called with proxyInfos",
			proxyInfoHistories:    make(ProxyInfoHistories),
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedActiveProxies: cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2},
			},
		},
		{
			name: "should update to histories if infoUID is different",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now()},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now()},
			},
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfoWithDifferentInfoUID},
			expectedActiveProxies: cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfoWithDifferentInfoUID},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfoWithDifferentInfoUID},
			},
		},
		{
			name: "should remove from active proxies if is not updated for twice the DefaultProxyTimeout",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now() - (td45ns)},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now() - (td15ns)},
			},
			proxiesInfos:          cluster.ProxiesInfo{pr1: &proxyInfo1, pr2: &proxyInfo2},
			expectedActiveProxies: cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2},
			},
		},
		{
			name: "should remove proxy from sentinel's local history if the proxy is removed in store",
			proxyInfoHistories: ProxyInfoHistories{
				pr1: &ProxyInfoHistory{ProxyInfo: &proxyInfo1, Timer: timer.Now()},
				pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2, Timer: timer.Now()},
			},
			proxiesInfos:               cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedActiveProxies:      cluster.ProxiesInfo{pr2: &proxyInfo2},
			expectedProxyInfoHistories: ProxyInfoHistories{pr2: &ProxyInfoHistory{ProxyInfo: &proxyInfo2}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Sentinel{
				uid:                "sentinel01",
				UIDFn:              testUIDFn,
				RandFn:             testRandFn,
				dbConvergenceInfos: map[string]*DBConvergenceInfo{},
				proxyInfoHistories: test.proxyInfoHistories,
			}
			actualActiveProxies := s.activeProxiesInfos(test.proxiesInfos)

			if !reflect.DeepEqual(actualActiveProxies, test.expectedActiveProxies) {
				t.Errorf("Expected proxiesInfos to be %v but got %v", test.expectedActiveProxies, actualActiveProxies)
			}
			if !isProxyInfoHistoriesEqual(s.proxyInfoHistories, test.expectedProxyInfoHistories) {
				t.Errorf("Expected proxyInfoHistories to be %v but got %v",
					test.expectedProxyInfoHistories, s.proxyInfoHistories)
			}
		})
	}
}

func isProxyInfoHistoriesEqual(actualProxyInfoHistories ProxyInfoHistories,
	expectedProxyInfoHistories ProxyInfoHistories) bool {
	if len(actualProxyInfoHistories) != len(expectedProxyInfoHistories) {
		return false
	}
	for k, expectedProxyInfoHistory := range expectedProxyInfoHistories {
		actualProxyInfoHistory, ok := actualProxyInfoHistories[k]
		if !ok {
			return false
		}
		if actualProxyInfoHistory.ProxyInfo.InfoUID != expectedProxyInfoHistory.ProxyInfo.InfoUID ||
			actualProxyInfoHistory.ProxyInfo.UID != expectedProxyInfoHistory.ProxyInfo.UID {
			return false
		}
	}
	return true
}

func testUIDFn() string {
	curUID++
	return fmt.Sprintf("%s%d", "db", curUID)
}

func testRandFn(_ int) int {
	return 0
}

func testEqualCD(cd1, cd2 *cluster.Data) bool {
	// ignore times
	for _, cd := range []*cluster.Data{cd1, cd2} {
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
