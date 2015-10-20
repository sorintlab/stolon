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

package cluster

import (
	"reflect"
	"sort"
	"time"
)

type KeepersState map[string]*KeeperState

func (kss KeepersState) Copy() KeepersState {
	nkss := KeepersState{}
	for k, v := range kss {
		nkss[k] = v.Copy()
	}
	return nkss
}

type KeeperState struct {
	ID                 string
	ErrorStartTime     time.Time
	Healthy            bool
	ClusterViewVersion int
	Host               string
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

func (ks *KeeperState) Changed(ki *KeeperInfo) bool {
	if ks.ClusterViewVersion != ki.ClusterViewVersion || ks.Host != ki.Host || ks.Port != ki.Port || ks.PGListenAddress != ki.PGListenAddress || ks.PGPort != ki.PGPort {
		return true
	}
	return false
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

type ClusterView struct {
	Version     int
	Master      string
	KeepersRole KeepersRole
	ChangeTime  time.Time
}

// NewClusterView return an initialized clusterView with Version: 0, zero
// ChangeTime, no Master and empty KeepersRole.
func NewClusterView() *ClusterView {
	return &ClusterView{
		KeepersRole: NewKeepersRole(),
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
		reflect.DeepEqual(cv.KeepersRole, ncv.KeepersRole)
}

func (cv *ClusterView) Copy() *ClusterView {
	if cv == nil {
		return nil
	}
	ncv := *cv
	ncv.KeepersRole = cv.KeepersRole.Copy()
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

// A struct contain the KeepersState and the ClusterView, needed to commit them togheter as etcd 2 doesn't supports multi key transactions
// TODO(sgotti) rework it when using etcd supports transactions.
type ClusterData struct {
	KeepersState KeepersState
	ClusterView  *ClusterView
}
