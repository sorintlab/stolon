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

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/stolon/pkg/cluster"
)

func TestUpdateClusterView(t *testing.T) {
	tests := []struct {
		cv           *cluster.ClusterView
		membersState cluster.MembersState
		outCV        *cluster.ClusterView
		err          error
	}{
		{
			cv:           cluster.NewClusterView(),
			membersState: nil,
			outCV:        cluster.NewClusterView(),
			err:          fmt.Errorf("cannot init cluster, no members registered"),
		},
		// cluster initialization, one member
		{
			cv: cluster.NewClusterView(),
			membersState: cluster.MembersState{
				"01": &cluster.MemberState{},
			},
			outCV: &cluster.ClusterView{
				Version: 1,
				Master:  "01",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
				},
			},
		},
		// cluster initialization, too many members
		{
			cv: cluster.NewClusterView(),
			membersState: cluster.MembersState{
				"01": &cluster.MemberState{},
				"02": &cluster.MemberState{},
			},
			outCV: cluster.NewClusterView(),
			err:   fmt.Errorf("cannot init cluster, more than 1 member registered"),
		},
		// One master and one standby, both healthy: no change from previous cv
		{
			cv: &cluster.ClusterView{
				Version: 1,
				Master:  "01",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: "01"},
				},
			},
			membersState: cluster.MembersState{
				"01": &cluster.MemberState{
					ClusterViewVersion: 1,
					ErrorStartTime:     time.Time{},
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.MemberState{
					ClusterViewVersion: 1,
					ErrorStartTime:     time.Time{},
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 1,
				Master:  "01",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: "01"},
				},
			},
		},
		// One master and one standby, master not healthy: standby elected as new master
		{
			cv: &cluster.ClusterView{
				Version: 1,
				Master:  "01",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: "01"},
				},
			},
			membersState: cluster.MembersState{
				"01": &cluster.MemberState{
					ClusterViewVersion: 1,
					ErrorStartTime:     time.Unix(0, 0),
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.MemberState{
					ClusterViewVersion: 1,
					ErrorStartTime:     time.Time{},
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 2,
				Master:  "02",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: ""},
				},
			},
		},
		// From the previous test, new master (02) converged. Old master setup to follow new master (02).
		{
			cv: &cluster.ClusterView{
				Version: 2,
				Master:  "02",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: ""},
				},
			},
			membersState: cluster.MembersState{
				"01": &cluster.MemberState{
					ClusterViewVersion: 1,
					ErrorStartTime:     time.Unix(0, 0),
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.MemberState{
					ClusterViewVersion: 2,
					ErrorStartTime:     time.Time{},
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 3,
				Master:  "02",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: "02"},
					"02": &cluster.MemberRole{Follow: ""},
				},
			},
		},

		// One master and one standby, master not healthy, standby with old
		// clusterview: no standby elected as new master.
		{
			cv: &cluster.ClusterView{
				Version: 2,
				Master:  "01",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: "01"},
				},
			},
			membersState: cluster.MembersState{
				"01": &cluster.MemberState{
					ClusterViewVersion: 2,
					ErrorStartTime:     time.Unix(0, 0),
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.MemberState{
					ClusterViewVersion: 1,
					ErrorStartTime:     time.Time{},
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 2,
				Master:  "01",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: "01"},
				},
			},
		},
		// One master and one standby, master not converged to current
		// cv: standby elected as new master.
		{
			cv: &cluster.ClusterView{
				Version: 2,
				Master:  "01",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: "01"},
				},
			},
			membersState: cluster.MembersState{
				"01": &cluster.MemberState{
					ClusterViewVersion: 1,
					ErrorStartTime:     time.Time{},
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
				"02": &cluster.MemberState{
					ClusterViewVersion: 2,
					ErrorStartTime:     time.Time{},
					PGState: &cluster.PostgresState{
						TimelineID: 0,
					},
				},
			},
			outCV: &cluster.ClusterView{
				Version: 3,
				Master:  "02",
				MembersRole: cluster.MembersRole{
					"01": &cluster.MemberRole{Follow: ""},
					"02": &cluster.MemberRole{Follow: ""},
				},
			},
		},
	}

	s := &Sentinel{id: "id", clusterConfig: cluster.NewDefaultConfig()}
	for i, tt := range tests {
		outCV, err := s.updateClusterView(tt.cv, tt.membersState)
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
