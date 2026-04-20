// Copyright 2026 PgVillage
// Copyright 2016 Sorint.lab
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

package v1

import (
	"reflect"
	"time"

	"github.com/mitchellh/copystructure"
)

// Data is meant to keep all the changes to the various components atomic (using
// an unique key)
type Data struct {
	// ClusterData format version. Used to detect incompatible
	// version and do upgrade. Needs to be bumped when a non
	// backward compatible change is done to the other struct
	// members.
	FormatVersion uint64    `json:"formatVersion"`
	ChangeTime    time.Time `json:"changeTime"`
	Cluster       *Cluster  `json:"cluster"`
	Keepers       Keepers   `json:"keepers"`
	DBs           DBs       `json:"dbs"`
	Proxy         *Proxy    `json:"proxy"`
}

// NewClusterData returns a freshly initialized Data object
func NewClusterData(c *Cluster) *Data {
	return &Data{
		FormatVersion: CurrentCDFormatVersion,
		Cluster:       c,
		Keepers:       Keepers{},
		DBs:           DBs{},
		Proxy:         &Proxy{},
	}
}

// DeepCopy copies the entire structure and returns a complete clone
func (d *Data) DeepCopy() (dc *Data) {
	var ok bool
	if nd, err := copystructure.Copy(d); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(d, nd) {
		panic("not equal")
	} else if dc, ok = nd.(*Data); !ok {
		panic("different type after copy")
	}
	return dc
}

// FindDB can be used to find a DB belonging to a Keeper
func (d *Data) FindDB(keeper *Keeper) *DB {
	for _, db := range d.DBs {
		if db.Spec.KeeperUID == keeper.UID {
			return db
		}
	}
	return nil
}
