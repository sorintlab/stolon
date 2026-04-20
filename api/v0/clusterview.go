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

// Package v0 holds the v0 api of the cluster info
package v0

import (
	"fmt"
	"reflect"
	"sort"
	"time"
)

const (
	// CurrentCDFormatVersion can be used to get the current version of a clusterdata
	CurrentCDFormatVersion uint64 = 0
)

// KeepersState holds the state of all keepers of this cluster
type KeepersState map[string]*KeeperState

// SortedKeys returns a sorted list of all keys of a KeepersState
func (kss KeepersState) SortedKeys() []string {
	keys := []string{}
	for k := range kss {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Copy will return a copy of a KeepersState
func (kss KeepersState) Copy() KeepersState {
	if kss == nil {
		return nil
	}
	nkss := KeepersState{}
	for k, v := range kss {
		nkss[k] = v.Copy()
	}
	return nkss
}

// NewFromKeeperInfo Initializes a KeoerInfo from a state
func (kss KeepersState) NewFromKeeperInfo(ki *KeeperInfo) error {
	id := ki.ID
	if _, ok := kss[id]; ok {
		return fmt.Errorf("keeperState with id %q already exists", id)
	}
	kss[id] = &KeeperState{
		ErrorStartTime:     time.Time{},
		ID:                 ki.ID,
		ClusterViewVersion: ki.ClusterViewVersion,
		ListenAddress:      ki.ListenAddress,
		Port:               ki.Port,
		PGListenAddress:    ki.PGListenAddress,
		PGPort:             ki.PGPort,
	}
	return nil
}

// KeeperState holds the state of one Keeper
type KeeperState struct {
	ID                 string
	ErrorStartTime     time.Time
	Healthy            bool
	ClusterViewVersion int
	ListenAddress      string
	Port               string
	PGListenAddress    string
	PGPort             string
	PGState            *PostgresState
}

// Copy returns a copy of a KeeprSate
func (ks *KeeperState) Copy() *KeeperState {
	if ks == nil {
		return nil
	}
	nks := *ks
	return &nks
}

// ChangedFromKeeperInfo returns true if a KeeperState has changed since it was defined for a Keeperinfo
func (ks *KeeperState) ChangedFromKeeperInfo(ki *KeeperInfo) (bool, error) {
	if ks.ID != ki.ID {
		return false, fmt.Errorf("different IDs, keeperState.ID: %s != keeperInfo.ID: %s", ks.ID, ki.ID)
	}
	if ks.ClusterViewVersion != ki.ClusterViewVersion ||
		ks.ListenAddress != ki.ListenAddress ||
		ks.Port != ki.Port ||
		ks.PGListenAddress != ki.PGListenAddress ||
		ks.PGPort != ki.PGPort {
		return true, nil
	}
	return false, nil
}

// UpdateFromKeeperInfo will update a KeeperState from a KeeperInfo
func (ks *KeeperState) UpdateFromKeeperInfo(ki *KeeperInfo) error {
	if ks.ID != ki.ID {
		return fmt.Errorf("different IDs, keeperState.ID: %s != keeperInfo.ID: %s", ks.ID, ki.ID)
	}
	ks.ClusterViewVersion = ki.ClusterViewVersion
	ks.ListenAddress = ki.ListenAddress
	ks.Port = ki.Port
	ks.PGListenAddress = ki.PGListenAddress
	ks.PGPort = ki.PGPort

	return nil
}

// SetError will set ErrorStartTime Which defines that an error has occurred and also when
func (ks *KeeperState) SetError() {
	if ks.ErrorStartTime.IsZero() {
		ks.ErrorStartTime = time.Now()
	}
}

// CleanError will clear ErrorStartTime
func (ks *KeeperState) CleanError() {
	ks.ErrorStartTime = time.Time{}
}

// KeepersRole is a map of KepperRole instances
type KeepersRole map[string]*KeeperRole

// GetFollowersIDs returns a sorted list of followersIDs
func (ksr KeepersRole) GetFollowersIDs(id string) []string {
	followersIDs := []string{}
	for keeperID, kr := range ksr {
		if kr.Follow == id {
			followersIDs = append(followersIDs, keeperID)
		}
	}
	sort.Strings(followersIDs)
	return followersIDs
}

// NewKeepersRole returns a new KeepersRole
func NewKeepersRole() KeepersRole {
	return KeepersRole{}
}

// Copy returns a copy of a KeepersRole
func (ksr KeepersRole) Copy() KeepersRole {
	if ksr == nil {
		return nil
	}
	nksr := KeepersRole{}
	for k, v := range ksr {
		nksr[k] = v.Copy()
	}
	return nksr
}

// Add is a safe method to add a role to a KeepersRole. It errors when it was already there
func (ksr KeepersRole) Add(id string, follow string) error {
	if _, ok := ksr[id]; ok {
		return fmt.Errorf("keeperRole with id %q already exists", id)
	}
	ksr[id] = &KeeperRole{ID: id, Follow: follow}
	return nil
}

// KeeperRole defines a role of a keeper, and what other keeper is is following
type KeeperRole struct {
	ID     string
	Follow string
}

// Copy returns a copy of a KeeperRole
func (kr *KeeperRole) Copy() *KeeperRole {
	if kr == nil {
		return nil
	}
	nkr := *kr
	return &nkr
}

// ProxyConf defines a proxy configuration which consists of a host and port
type ProxyConf struct {
	Host string
	Port string
}

// Copy  returns a copy of a ProxyConf
func (pc *ProxyConf) Copy() *ProxyConf {
	if pc == nil {
		return nil
	}
	npc := *pc
	return &npc
}

// ClusterView defines an overview of a cluster
type ClusterView struct {
	Version     int
	Master      string
	KeepersRole KeepersRole
	ProxyConf   *ProxyConf
	Config      *NilConfig
	ChangeTime  time.Time
}

// NewClusterView return an initialized clusterView with Version: 0, zero
// ChangeTime, no Master and empty KeepersRole.
func NewClusterView() *ClusterView {
	return &ClusterView{
		KeepersRole: NewKeepersRole(),
		Config:      &NilConfig{},
	}
}

// Equals checks if the clusterViews are the same. It ignores the ChangeTime.
func (cv *ClusterView) Equals(ncv *ClusterView) bool {
	if cv == nil {
		return ncv == nil
	}
	return cv.Version == ncv.Version &&
		cv.Master == ncv.Master &&
		reflect.DeepEqual(cv.KeepersRole, ncv.KeepersRole) &&
		reflect.DeepEqual(cv.ProxyConf, ncv.ProxyConf) &&
		reflect.DeepEqual(cv.Config, ncv.Config)
}

// Copy returns a copy of a ClusterView
func (cv *ClusterView) Copy() *ClusterView {
	if cv == nil {
		return nil
	}
	ncv := *cv
	ncv.KeepersRole = cv.KeepersRole.Copy()
	ncv.ProxyConf = cv.ProxyConf.Copy()
	ncv.Config = cv.Config.Copy()
	ncv.ChangeTime = cv.ChangeTime
	return &ncv
}

// GetFollowersIDs returns a sorted list of followersIDs
func (cv *ClusterView) GetFollowersIDs(id string) []string {
	return cv.KeepersRole.GetFollowersIDs(id)
}

// ClusterData defines a struct containing the KeepersState and the ClusterView since they need to be in sync
type ClusterData struct {
	// ClusterData format version. Used to detect incompatible
	// version and do upgrade. Needs to be bumped when a non
	// backward compatible change is done to the other struct
	// members.
	FormatVersion uint64

	KeepersState KeepersState
	ClusterView  *ClusterView
}
