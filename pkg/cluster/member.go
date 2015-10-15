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

import "github.com/sorintlab/stolon/common"

type Member struct {
	ClusterViewVersion int
}

type MembersInfo map[string]*MemberInfo

type MemberInfo struct {
	ID                 string
	ClusterViewVersion int
	Host               string
	Port               string
	PGListenAddress    string
	PGPort             string
}

func (m *MemberInfo) Copy() *MemberInfo {
	nm := *m
	return &nm
}

func (mi *MemberInfo) Changed(m *MemberState) bool {
	if m.ClusterViewVersion != mi.ClusterViewVersion || m.Host != mi.Host || m.Port != mi.Port || m.PGListenAddress != mi.PGListenAddress || m.PGPort != mi.PGPort {
		return true
	}
	return false
}

type PostgresTimeLineHistory struct {
	TimelineID  uint64
	SwitchPoint uint64
	Reason      string
}
type PostgresState struct {
	Initialized     bool
	Role            common.Role
	SystemID        string
	TimelineID      uint64
	XLogPos         uint64
	TimelineHistory []PostgresTimeLineHistory
}

type MembersDiscoveryInfo []*MemberDiscoveryInfo

type MemberDiscoveryInfo struct {
	Host string
	Port string
}
