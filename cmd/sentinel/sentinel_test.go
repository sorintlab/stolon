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
	"reflect"
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
	}{
		{
			cv:           nil,
			membersState: nil,
			outCV:        nil,
		},
		// cluster initialization, one member
		{
			cv: nil,
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
			cv: nil,
			membersState: cluster.MembersState{
				"01": &cluster.MemberState{},
				"02": &cluster.MemberState{},
			},
			outCV: nil,
		},
		// One master and one slave, both healthy: no change from previous cv
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
			outCV: nil,
		},
		// One master and one slave, master not healthy: slave elected as new master
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
					"01": &cluster.MemberRole{Follow: "02"},
					"02": &cluster.MemberRole{Follow: ""},
				},
			},
		},
		// One master and one slave, master not healthy, slave with old
		// clusterview: no slave elected as new master.
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
			outCV: nil,
		},
		// One master and one slave, master not converged to current
		// cv: slave elected as new master.
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
					"01": &cluster.MemberRole{Follow: "02"},
					"02": &cluster.MemberRole{Follow: ""},
				},
			},
		},
	}

	cfg := config{}
	s := NewSentinel("id", cfg, nil, nil)
	for i, tt := range tests {
		outCV := s.updateClusterView(tt.cv, tt.membersState)
		t.Logf("test #%d", i)
		t.Logf(spew.Sprintf("outCV: %#v", outCV))
		if !reflect.DeepEqual(tt.outCV, outCV) {
			t.Errorf(spew.Sprintf("#%d: wrong outCV: got: %#v, want: %#v", i, outCV, tt.outCV))
		}
	}
}
