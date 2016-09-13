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
	"github.com/sorintlab/stolon/pkg/cluster"
)

func TestUpdateClusterView(t *testing.T) {
	tests := []struct {
		cv           *cluster.ClusterView
		keepersState cluster.KeepersState
		outCV        *cluster.ClusterView
		err          error
	}{
		{
			cv:           cluster.NewClusterView(),
			keepersState: nil,
			outCV:        cluster.NewClusterView(),
			err:          fmt.Errorf("cluster view at version 0 without a defined master. This shouldn't happen!"),
		},
		{
			cv: &cluster.ClusterView{
				Version:     1,
				KeepersRole: cluster.KeepersRole{},
			},
			keepersState: nil,
			outCV: &cluster.ClusterView{
				Version:     1,
				KeepersRole: cluster.KeepersRole{},
			},
			err: fmt.Errorf("cannot choose initial master, no keepers registered"),
		},
		// cluster initialization, one keeper
		{
			cv: &cluster.ClusterView{
				Version:     1,
				KeepersRole: cluster.KeepersRole{},
			},
			keepersState: cluster.KeepersState{
				"01": &cluster.KeeperState{PGState: &cluster.PostgresState{Initialized: true}},
			},
			outCV: &cluster.ClusterView{
				Version: 2,
				Master:  "01",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
				},
			},
		},
		// cluster initialization, too many keepers
		{
			cv: &cluster.ClusterView{
				Version:     1,
				KeepersRole: cluster.KeepersRole{},
			},
			keepersState: cluster.KeepersState{
				"01": &cluster.KeeperState{},
				"02": &cluster.KeeperState{},
			},
			outCV: &cluster.ClusterView{
				Version:     1,
				KeepersRole: cluster.KeepersRole{},
			},
			err: fmt.Errorf("cannot choose initial master, more than 1 keeper registered"),
		},
		// cluster initialization, more then one keeper but InitWithMultipleKeepers == true
		{
			cv: &cluster.ClusterView{
				Version:     1,
				KeepersRole: cluster.KeepersRole{},
				Config:      &cluster.NilConfig{InitWithMultipleKeepers: cluster.BoolP(true)},
			},
			keepersState: cluster.KeepersState{
				"01": &cluster.KeeperState{PGState: &cluster.PostgresState{Initialized: true}},
				"02": &cluster.KeeperState{PGState: &cluster.PostgresState{Initialized: true}},
			},
			outCV: &cluster.ClusterView{
				Version: 2,
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: ""},
				},
				Config: &cluster.NilConfig{InitWithMultipleKeepers: cluster.BoolP(true)},
			},
		},

		// One master and one standby, both healthy: no change from previous cv
		{
			cv: &cluster.ClusterView{
				Version: 1,
				Master:  "01",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: "01"},
				},
				ProxyConf: &cluster.ProxyConf{Host: "01", Port: "01"},
			},
			keepersState: cluster.KeepersState{
				"01": &cluster.KeeperState{
					ClusterViewVersion: 1,
					PGListenAddress:    "01",
					PGPort:             "01",
					ErrorStartTime:     time.Time{},
					Healthy:            true,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.KeeperState{
					ClusterViewVersion: 1,
					PGListenAddress:    "02",
					PGPort:             "02",
					ErrorStartTime:     time.Time{},
					Healthy:            true,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 1,
				Master:  "01",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: "01"},
				},
				ProxyConf: &cluster.ProxyConf{Host: "01", Port: "01"},
			},
		},
		// One master and one standby, master not healthy: standby elected as new master
		{
			cv: &cluster.ClusterView{
				Version: 1,
				Master:  "01",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: "01"},
				},
				ProxyConf: &cluster.ProxyConf{Host: "01", Port: "01"},
			},
			keepersState: cluster.KeepersState{
				"01": &cluster.KeeperState{
					ClusterViewVersion: 1,
					PGListenAddress:    "01",
					PGPort:             "01",
					ErrorStartTime:     time.Unix(0, 0),
					Healthy:            false,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.KeeperState{
					ClusterViewVersion: 1,
					PGListenAddress:    "02",
					PGPort:             "02",
					ErrorStartTime:     time.Time{},
					Healthy:            true,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 2,
				Master:  "02",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: ""},
				},
				ProxyConf: nil,
			},
		},
		// From the previous test, new master (02) converged. Old master setup to follow new master (02).
		{
			cv: &cluster.ClusterView{
				Version: 2,
				Master:  "02",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: ""},
				},
				ProxyConf: nil,
			},
			keepersState: cluster.KeepersState{
				"01": &cluster.KeeperState{
					ClusterViewVersion: 1,
					PGListenAddress:    "01",
					PGPort:             "01",
					ErrorStartTime:     time.Unix(0, 0),
					Healthy:            false,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.KeeperState{
					ClusterViewVersion: 2,
					PGListenAddress:    "02",
					PGPort:             "02",
					ErrorStartTime:     time.Time{},
					Healthy:            true,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 3,
				Master:  "02",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: "02"},
					"02": &cluster.KeeperRole{ID: "02", Follow: ""},
				},
				ProxyConf: &cluster.ProxyConf{Host: "02", Port: "02"},
			},
		},

		// One master and one standby, master not healthy, standby with old
		// clusterview: no standby elected as new master.
		{
			cv: &cluster.ClusterView{
				Version: 2,
				Master:  "01",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: "01"},
				},
				ProxyConf: &cluster.ProxyConf{Host: "01", Port: "01"},
			},
			keepersState: cluster.KeepersState{
				"01": &cluster.KeeperState{
					ClusterViewVersion: 2,
					PGListenAddress:    "01",
					PGPort:             "01",
					ErrorStartTime:     time.Unix(0, 0),
					Healthy:            true,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.KeeperState{
					ClusterViewVersion: 1,
					PGListenAddress:    "02",
					PGPort:             "02",
					ErrorStartTime:     time.Time{},
					Healthy:            true,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 2,
				Master:  "01",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: "01"},
				},
				ProxyConf: &cluster.ProxyConf{Host: "01", Port: "01"},
			},
		},
		// One master and one standby, master not converged to current
		// cv: standby elected as new master.
		{
			cv: &cluster.ClusterView{
				Version: 2,
				Master:  "01",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: "01"},
				},
				ProxyConf: nil,
			},
			keepersState: cluster.KeepersState{
				"01": &cluster.KeeperState{
					ClusterViewVersion: 1,
					PGListenAddress:    "01",
					PGPort:             "01",
					ErrorStartTime:     time.Time{},
					Healthy:            true,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.KeeperState{
					ClusterViewVersion: 2,
					PGListenAddress:    "02",
					PGPort:             "02",
					ErrorStartTime:     time.Time{},
					Healthy:            true,
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 3,
				Master:  "02",
				KeepersRole: cluster.KeepersRole{
					"01": &cluster.KeeperRole{ID: "01", Follow: ""},
					"02": &cluster.KeeperRole{ID: "02", Follow: ""},
				},
				ProxyConf: nil,
			},
		},
	}

	for i, tt := range tests {
		var s *Sentinel
		if tt.cv.Config == nil {
			s = &Sentinel{id: "id", clusterConfig: cluster.NewDefaultConfig()}
		} else {
			s = &Sentinel{id: "id", clusterConfig: tt.cv.Config.ToConfig()}
		}
		outCV, err := s.updateClusterView(tt.cv, tt.keepersState)
		t.Logf("test #%d", i)
		t.Logf(spew.Sprintf("outCV: %#v", outCV))
		if tt.err != nil {
			if err == nil {
				t.Errorf("got no error, wanted error: %v", tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("got error: %v, wanted error: %v", err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !outCV.Equals(tt.outCV) {
				t.Errorf(spew.Sprintf("#%d: wrong outCV: got: %#v, want: %#v", i, outCV, tt.outCV))
			}
		}
	}
}
