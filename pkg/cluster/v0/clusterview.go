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

package v0

import (
	"fmt"
	"reflect"
	"sort"
	"time"
)

const (
	CurrentCDFormatVersion uint64 = 0
)

type KeepersState map[string]*KeeperState

func (kss KeepersState) SortedKeys() []string {
	keys := []string{}
	for k, _ := range kss {
		keys = append(keys, k)
	}
	sort.Sort(sort.StringSlice(keys))
	return keys
}

func (kss KeepersState) Copy() KeepersState {
	nkss := KeepersState{}
	for k, v := range kss {
		nkss[k] = v.Copy()
	}
	return nkss
}

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

func (ks *KeeperState) Copy() *KeeperState {
	if ks == nil {
		return nil
	}
	nks := *ks
	return &nks
}

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

func (ks *KeeperState) SetError() {
	if ks.ErrorStartTime.IsZero() {
		ks.ErrorStartTime = time.Now()
	}
}

func (ks *KeeperState) CleanError() {
	ks.ErrorStartTime = time.Time{}
}

type KeepersRole map[string]*KeeperRole

func NewKeepersRole() KeepersRole {
	return make(KeepersRole)
}

func (ksr KeepersRole) Copy() KeepersRole {
	nksr := KeepersRole{}
	for k, v := range ksr {
		nksr[k] = v.Copy()
	}
	return nksr
}

func (ksr KeepersRole) Add(id string, follow string) error {
	if _, ok := ksr[id]; ok {
		return fmt.Errorf("keeperRole with id %q already exists", id)
	}
	ksr[id] = &KeeperRole{ID: id, Follow: follow}
	return nil
}

type KeeperRole struct {
	ID     string
	Follow string
}

func (kr *KeeperRole) Copy() *KeeperRole {
	if kr == nil {
		return nil
	}
	nkr := *kr
	return &nkr
}

type ProxyConf struct {
	Host string
	Port string
}

func (pc *ProxyConf) Copy() *ProxyConf {
	if pc == nil {
		return nil
	}
	npc := *pc
	return &npc
}

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
		if ncv == nil {
			return true
		}
		return false
	}
	return cv.Version == ncv.Version &&
		cv.Master == cv.Master &&
		reflect.DeepEqual(cv.KeepersRole, ncv.KeepersRole) &&
		reflect.DeepEqual(cv.ProxyConf, ncv.ProxyConf) &&
		reflect.DeepEqual(cv.Config, ncv.Config)
}

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

// Returns a sorted list of followersIDs
func (cv *ClusterView) GetFollowersIDs(id string) []string {
	followersIDs := []string{}
	for keeperID, kr := range cv.KeepersRole {
		if kr.Follow == id {
			followersIDs = append(followersIDs, keeperID)
		}
	}
	sort.Strings(followersIDs)
	return followersIDs
}

// A struct containing the KeepersState and the ClusterView since they need to be in sync
type ClusterData struct {
	// ClusterData format version. Used to detect incompatible
	// version and do upgrade. Needs to be bumped when a non
	// backward compatible change is done to the other struct
	// members.
	FormatVersion uint64

	KeepersState KeepersState
	ClusterView  *ClusterView
}
