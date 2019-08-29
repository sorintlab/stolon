// Copyright 2018 Sorint.lab
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

package cluster

import (
	"errors"
	"testing"
)

func TestValidateReplicationSlots(t *testing.T) {
	tests := []struct {
		in  string
		err error
	}{
		{
			in: "goodslotname_434432",
		},
		{
			in:  "badslotname-34223",
			err: errors.New(`wrong replication slot name: "badslotname-34223"`),
		},
		{
			in:  "badslotname\n",
			err: errors.New(`wrong replication slot name: "badslotname\n"`),
		},
		{
			in:  " badslotname",
			err: errors.New(`wrong replication slot name: " badslotname"`),
		},
		{
			in:  "badslotname ",
			err: errors.New(`wrong replication slot name: "badslotname "`),
		},
		{
			in:  "stolon_c874a3cb",
			err: errors.New(`replication slot name is reserved: "stolon_c874a3cb"`),
		},
	}

	for i, tt := range tests {
		err := validateReplicationSlot(tt.in)

		if tt.err != nil {
			if err == nil {
				t.Errorf("#%d: got no error, wanted error: %v", i, tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("#%d: got error: %v, wanted error: %v", i, err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("#%d: unexpected error: %v", i, err)
			}
		}
	}
}

func TestClusterData_FindDB(t *testing.T) {
	db := DB{
		UID:  "dbUUID",
		Spec: &DBSpec{KeeperUID: "sameKeeperUUID"},
	}
	tests := []struct {
		name        string
		clusterData ClusterData
		keeper      *Keeper
		expectedDB  *DB
	}{
		{
			name:        "should return nil if the clusterData is empty",
			clusterData: ClusterData{},
			keeper:      &Keeper{},
			expectedDB:  nil,
		},
		{
			name: "should return nil if DB is not found for given keeper",
			clusterData: ClusterData{
				DBs: map[string]*DB{
					"dbUUID": &db,
				},
			},
			keeper:     &Keeper{UID: "differentUUID"},
			expectedDB: nil,
		},
		{
			name: "should return the DB if DB is found for given keeper",
			clusterData: ClusterData{
				DBs: map[string]*DB{
					"dbUUID": &db,
				},
			},
			keeper:     &Keeper{UID: "sameKeeperUUID"},
			expectedDB: &db,
		},
	}

	for _, tt := range tests {
		actual := tt.clusterData.FindDB(tt.keeper)
		if actual != tt.expectedDB {
			t.Errorf("Expected %v, but got %v", tt.expectedDB, actual)
		}
	}
}
