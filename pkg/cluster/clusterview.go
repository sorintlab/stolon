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
	"time"
)

type MembersState map[string]*MemberState

func (mss MembersState) Copy() MembersState {
	nmss := MembersState{}
	for k, v := range mss {
		nmss[k] = v.Copy()
	}
	return nmss
}

type MemberState struct {
	ErrorStartTime     time.Time
	ClusterViewVersion int
	Host               string
	Port               string
	PGListenAddress    string
	PGPort             string
	PGState            *PostgresState
}

func (m *MemberState) Copy() *MemberState {
	if m == nil {
		return nil
	}
	nm := *m
	return &nm
}

func (m *MemberState) Changed(mi *MemberInfo) bool {
	if m.ClusterViewVersion != mi.ClusterViewVersion || m.Host != mi.Host || m.Port != mi.Port || m.PGListenAddress != mi.PGListenAddress || m.PGPort != mi.PGPort {
		return true
	}
	return false
}

func (m *MemberState) MarkError() {
	if m.ErrorStartTime.IsZero() {
		m.ErrorStartTime = time.Now()
	}
}

func (m *MemberState) MarkOK() {
	m.ErrorStartTime = time.Time{}
}

type MembersRole map[string]*MemberRole

func NewMembersRole() MembersRole {
	return make(MembersRole)
}

func (msr MembersRole) Copy() MembersRole {
	nmsr := MembersRole{}
	for k, v := range msr {
		nmsr[k] = v.Copy()
	}
	return nmsr
}

type MemberRole struct {
	Follow string
}

func (mr *MemberRole) Copy() *MemberRole {
	if mr == nil {
		return nil
	}
	nmr := *mr
	return &nmr
}

type ClusterView struct {
	Version     int
	Master      string
	MembersRole MembersRole
	ChangeTime  time.Time
}

// NewClusterView return an initialized clusterView with Version: 0, zero
// ChangeTime, no Master and empty MembersRole.
func NewClusterView() *ClusterView {
	return &ClusterView{
		MembersRole: NewMembersRole(),
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
		reflect.DeepEqual(cv.MembersRole, ncv.MembersRole)
}

func (cv *ClusterView) Copy() *ClusterView {
	if cv == nil {
		return nil
	}
	ncv := *cv
	ncv.MembersRole = cv.MembersRole.Copy()
	return &ncv
}

func (cv *ClusterView) GetFollowersIDs(id string) []string {
	followersIDs := []string{}
	for memberID, mr := range cv.MembersRole {
		if mr.Follow == id {
			followersIDs = append(followersIDs, memberID)
		}
	}
	return followersIDs
}

// A struct contain the MembersState and the ClusterView, needed to commit them togheter as etcd 2 doesn't supports multi key transactions
// TODO(sgotti) rework it when using etcd supports transactions.
type ClusterData struct {
	MembersState MembersState
	ClusterView  *ClusterView
}
