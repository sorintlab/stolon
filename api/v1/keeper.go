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
	"sort"
	"time"

	"github.com/pgvillage-tools/stolon/internal/util"
)

// KeeperSpec defines a spec for a Keeper resource
type KeeperSpec struct{}

// KeeperStatus defines all staus fields on a Keeper
type KeeperStatus struct {
	Healthy         bool      `json:"healthy,omitempty"`
	LastHealthyTime time.Time `json:"lastHealthyTime,omitempty"`

	BootUUID string `json:"bootUUID,omitempty"`

	PostgresBinaryVersion PostgresBinaryVersion `json:"postgresBinaryVersion,omitempty"`

	ForceFail bool `json:"forceFail,omitempty"`

	CanBeMaster             *bool `json:"canBeMaster,omitempty"`
	CanBeSynchronousReplica *bool `json:"canBeSynchronousReplica,omitempty"`
}

// Keeper combines the spec, status and other fields belonging to a keeper
type Keeper struct {
	// Keeper ID
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *KeeperSpec `json:"spec,omitempty"`

	Status KeeperStatus `json:"status,omitempty"`
}

// NewKeeperFromKeeperInfo returns a freshly initialized keeper created from a KeeperInfo object
func NewKeeperFromKeeperInfo(ki *KeeperInfo) *Keeper {
	return &Keeper{
		UID:        ki.UID,
		Generation: InitialGeneration,
		ChangeTime: time.Time{},
		Spec:       &KeeperSpec{},
		Status: KeeperStatus{
			Healthy:         true,
			LastHealthyTime: time.Now(),
			BootUUID:        ki.BootUUID,
		},
	}
}

// Keepers can store all keepers for a cluster
type Keepers map[string]*Keeper

// SortedKeys returns all keys of the Keepers as a sorted list
func (kss Keepers) SortedKeys() []string {
	keys := []string{}
	for k := range kss {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// KeepersInfo stores all KeeperInfo resources belonging to this cluster
type KeepersInfo map[string]*KeeperInfo

// DeepCopy returns a copy of the KeepersInfo resource
func (k *KeepersInfo) DeepCopy() (dc *KeepersInfo) {
	return util.DeepCopy(k)
}

// KeeperInfo can store all info belonging to a Keeper
type KeeperInfo struct {
	// An unique id for this info, used to know when this the keeper info
	// has been updated
	InfoUID string `json:"infoUID,omitempty"`

	UID        string `json:"uid,omitempty"`
	ClusterUID string `json:"clusterUID,omitempty"`
	BootUUID   string `json:"bootUUID,omitempty"`

	PostgresBinaryVersion PostgresBinaryVersion `json:"postgresBinaryVersion,omitempty"`

	PostgresState *PostgresState `json:"postgresState,omitempty"`

	CanBeMaster             *bool `json:"canBeMaster,omitempty"`
	CanBeSynchronousReplica *bool `json:"canBeSynchronousReplica,omitempty"`
}

// DeepCopy returns a copy of the KeeperInfo resource
func (k *KeeperInfo) DeepCopy() (dc *KeeperInfo) {
	return util.DeepCopy(k)
}
