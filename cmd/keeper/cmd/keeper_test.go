// Copyright 2017 Sorint.lab
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
	"bytes"
	"errors"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/sorintlab/stolon/internal/mock/postgresql"
	"github.com/sorintlab/stolon/internal/postgresql"
	"reflect"
	"testing"

	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/common"
)

var curUID int

func TestParseSynchronousStandbyNames(t *testing.T) {
	tests := []struct {
		in  string
		out []string
		err error
	}{
		{
			in:  "2 (stolon_2c3870f3,stolon_c874a3cb)",
			out: []string{"stolon_2c3870f3", "stolon_c874a3cb"},
		},
		{
			in:  "2 ( stolon_2c3870f3 , stolon_c874a3cb )",
			out: []string{"stolon_2c3870f3", "stolon_c874a3cb"},
		},
		{
			in:  "21 (\" stolon_2c3870f3\",stolon_c874a3cb)",
			out: []string{"\" stolon_2c3870f3\"", "stolon_c874a3cb"},
		},
		{
			in:  "stolon_2c3870f3,stolon_c874a3cb",
			out: []string{"stolon_2c3870f3", "stolon_c874a3cb"},
		},
		{
			in:  "node1",
			out: []string{"node1"},
		},
		{
			in:  "2 (node1,",
			out: []string{"node1"},
			err: errors.New("synchronous standby string has number but lacks brackets"),
		},
	}

	for i, tt := range tests {
		out, err := parseSynchronousStandbyNames(tt.in)

		if tt.err != nil {
			if err == nil {
				t.Errorf("%d: got no error, wanted error: %v", i, tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("%d: got error: %v, wanted error: %v", i, err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("%d: unexpected error: %v", i, err)
			} else if !reflect.DeepEqual(out, tt.out) {
				t.Errorf("%d: wrong output: got:\n%s\nwant:\n%s", i, out, tt.out)
			}
		}
	}
}

func TestGenerateHBA(t *testing.T) {
	// minimal clusterdata with only the fields used by generateHBA
	cd := &cluster.ClusterData{
		Cluster: &cluster.Cluster{
			Spec:   &cluster.ClusterSpec{},
			Status: cluster.ClusterStatus{},
		},
		Keepers: cluster.Keepers{},
		DBs: cluster.DBs{
			"db1": &cluster.DB{
				UID: "db1",
				Spec: &cluster.DBSpec{
					Role: common.RoleMaster,
				},
				Status: cluster.DBStatus{
					ListenAddress: "192.168.0.1",
				},
			},
			"db2": &cluster.DB{
				UID: "db2",
				Spec: &cluster.DBSpec{
					Role: common.RoleStandby,
					FollowConfig: &cluster.FollowConfig{
						Type:  cluster.FollowTypeInternal,
						DBUID: "db1",
					},
				},
				Status: cluster.DBStatus{
					ListenAddress: "192.168.0.2",
				},
			},
			"db3": &cluster.DB{
				UID: "db3",
				Spec: &cluster.DBSpec{
					Role: common.RoleStandby,
					FollowConfig: &cluster.FollowConfig{
						Type:  cluster.FollowTypeInternal,
						DBUID: "db1",
					},
				},
				Status: cluster.DBStatus{
					ListenAddress: "192.168.0.3",
				},
			},
		},
		Proxy: &cluster.Proxy{},
	}

	tests := []struct {
		DefaultSUReplAccessMode cluster.SUReplAccessMode
		dbUID                   string
		pgHBA                   []string
		out                     []string
	}{
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessAll,
			dbUID:                   "db1",
			out: []string{
				"local postgres superuser md5",
				"local replication repluser md5",
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessAll,
			dbUID:                   "db2",
			out: []string{
				"local postgres superuser md5",
				"local replication repluser md5",
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessAll,
			dbUID:                   "db1",
			pgHBA: []string{
				"host all all 192.168.0.0/24 md5",
			},
			out: []string{
				"local postgres superuser md5",
				"local replication repluser md5",
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 192.168.0.0/24 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessAll,
			dbUID:                   "db2",
			pgHBA: []string{
				"host all all 192.168.0.0/24 md5",
			},
			out: []string{
				"local postgres superuser md5",
				"local replication repluser md5",
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 192.168.0.0/24 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessStrict,
			dbUID:                   "db1",
			out: []string{
				"local postgres superuser md5",
				"local replication repluser md5",
				"host all superuser 192.168.0.2/32 md5",
				"host replication repluser 192.168.0.2/32 md5",
				"host all superuser 192.168.0.3/32 md5",
				"host replication repluser 192.168.0.3/32 md5",
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessStrict,
			dbUID:                   "db2",
			out: []string{
				"local postgres superuser md5",
				"local replication repluser md5",
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			},
		},
	}

	for i, tt := range tests {
		p := &PostgresKeeper{
			pgSUAuthMethod:   "md5",
			pgSUUsername:     "superuser",
			pgReplAuthMethod: "md5",
			pgReplUsername:   "repluser",
		}

		cd.Cluster.Spec.DefaultSUReplAccessMode = &tt.DefaultSUReplAccessMode

		db := cd.DBs[tt.dbUID]
		db.Spec.PGHBA = tt.pgHBA

		out := p.generateHBA(cd, db, false)

		if !reflect.DeepEqual(out, tt.out) {
			var b bytes.Buffer
			b.WriteString(fmt.Sprintf("#%d: wrong output: got:\n", i))
			for _, o := range out {
				b.WriteString(fmt.Sprintf("%s\n", o))
			}
			b.WriteString(fmt.Sprintf("\nwant:\n"))
			for _, o := range tt.out {
				b.WriteString(fmt.Sprintf("%s\n", o))
			}
			t.Errorf(b.String())
		}
	}
}

func TestGetTimeLinesHistory(t *testing.T) {
	t.Run("should return empty if timelineID is not greater than 1", func(t *testing.T) {
		pgState := &cluster.PostgresState{
			TimelineID: 1,
		}

		ctlsh, err := getTimeLinesHistory(pgState, nil, 3)

		if err != nil {
			t.Errorf("err should be nil, but got %v", err)
		}
		if len(ctlsh) != 0 {
			t.Errorf("expecting empty ctlsh, but got %v", ctlsh)
		}
	})

	t.Run("should return error if there is error in getting timeline history", func(t *testing.T) {
		var timelineID uint64 = 2
		pgState := &cluster.PostgresState{
			TimelineID: timelineID,
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pgm := mocks.NewMockPGManager(ctrl)
		pgm.EXPECT().GetTimelinesHistory(timelineID).Return([]*postgresql.TimelineHistory{}, fmt.Errorf("failed to get timeline history"))
		ctlsh, err := getTimeLinesHistory(pgState, pgm, 3)

		if err == nil {
			t.Errorf("err should be not be nil")
		}
		if err != nil && err.Error() != "failed to get timeline history" {
			t.Errorf("err should be failed to get timeline history, but got %v", err)
		}
		if len(ctlsh) != 0 {
			t.Errorf("expecting empty ctlsh, but got %v", ctlsh)
		}
	})

	t.Run("should return timeline history as is if the given length is less than maxPostgresTimelinesHistory", func(t *testing.T) {
		var timelineID uint64 = 2
		pgState := &cluster.PostgresState{
			TimelineID: timelineID,
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pgm := mocks.NewMockPGManager(ctrl)
		timelineHistories := []*postgresql.TimelineHistory{
			{
				TimelineID:  1,
				SwitchPoint: 1,
				Reason:      "reason1",
			},
			{
				TimelineID:  2,
				SwitchPoint: 2,
				Reason:      "reason2",
			},
		}
		pgm.EXPECT().GetTimelinesHistory(timelineID).Return(timelineHistories, nil)
		ctlsh, err := getTimeLinesHistory(pgState, pgm, 3)

		if err != nil {
			t.Errorf("err should be not be nil")
		}
		if len(ctlsh) != 2 {
			t.Errorf("expecting length of ctlsh to be 2, but got %d", len(ctlsh))
		}
		expectedTimelineHistories := cluster.PostgresTimelinesHistory{
			&cluster.PostgresTimelineHistory{
				TimelineID:  1,
				SwitchPoint: 1,
				Reason:      "reason1",
			},
			&cluster.PostgresTimelineHistory{
				TimelineID:  2,
				SwitchPoint: 2,
				Reason:      "reason2",
			},
		}
		fmt.Println(ctlsh, expectedTimelineHistories)
		if *ctlsh[0] != *expectedTimelineHistories[0] {
			t.Errorf("expected %v, but got %v ", *expectedTimelineHistories[0], *ctlsh[0])
		}
		if *ctlsh[1] != *expectedTimelineHistories[1] {
			t.Errorf("expected %v, but got %v ", *expectedTimelineHistories[1], *ctlsh[1])
		}
	})

	t.Run("should return timeline history with last maxPostgresTimelinesHistory elements if timeline history length is greater than maxPostgresTimelinesHistory", func(t *testing.T) {
		var timelineID uint64 = 4
		pgState := &cluster.PostgresState{
			TimelineID: timelineID,
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pgm := mocks.NewMockPGManager(ctrl)
		timelineHistories := []*postgresql.TimelineHistory{
			{
				TimelineID:  1,
				SwitchPoint: 1,
				Reason:      "reason1",
			},
			{
				TimelineID:  2,
				SwitchPoint: 2,
				Reason:      "reason2",
			},
			{
				TimelineID:  3,
				SwitchPoint: 3,
				Reason:      "reason3",
			},
			{
				TimelineID:  4,
				SwitchPoint: 4,
				Reason:      "reason4",
			},
		}
		pgm.EXPECT().GetTimelinesHistory(timelineID).Return(timelineHistories, nil)
		ctlsh, err := getTimeLinesHistory(pgState, pgm, 2)

		if err != nil {
			t.Errorf("err should be not be nil")
		}
		if len(ctlsh) != 2 {
			t.Errorf("expecting length of ctlsh to be 2, but got %d", len(ctlsh))
		}
		expectedTimelineHistories := cluster.PostgresTimelinesHistory{
			&cluster.PostgresTimelineHistory{
				TimelineID:  3,
				SwitchPoint: 3,
				Reason:      "reason3",
			},
			&cluster.PostgresTimelineHistory{
				TimelineID:  4,
				SwitchPoint: 4,
				Reason:      "reason4",
			},
		}
		fmt.Println(ctlsh, expectedTimelineHistories)
		if *ctlsh[0] != *expectedTimelineHistories[0] {
			t.Errorf("expected %v, but got %v ", *expectedTimelineHistories[0], *ctlsh[0])
		}
		if *ctlsh[1] != *expectedTimelineHistories[1] {
			t.Errorf("expected %v, but got %v ", *expectedTimelineHistories[1], *ctlsh[1])
		}
	})
}
