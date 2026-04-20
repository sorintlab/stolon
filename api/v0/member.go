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

package v0

// TODO: Remove duplication with internal/cluster/member.go

import "github.com/pgvillage-tools/stolon/internal/common"

// KeepersInfo stores all info on all keepers belonging to this cluster
type KeepersInfo map[string]*KeeperInfo

// KeeperInfo stores all info on one keeper
type KeeperInfo struct {
	ID                 string
	ClusterViewVersion int
	ListenAddress      string
	Port               string
	PGListenAddress    string
	PGPort             string
}

// Copy returns a shallow copy
func (k *KeeperInfo) Copy() *KeeperInfo {
	if k == nil {
		return nil
	}
	nk := *k
	return &nk
}

// PostgresTimelinesHistory stores all PostgreSQL timelines belonging to this cluster
type PostgresTimelinesHistory []*PostgresTimelineHistory

// Copy returns a shallow copy
func (tlsh *PostgresTimelinesHistory) Copy() PostgresTimelinesHistory {
	if tlsh == nil {
		return nil
	}
	var ntlsh PostgresTimelinesHistory
	for _, ptlh := range *tlsh {
		ntlsh = append(ntlsh, &PostgresTimelineHistory{
			TimelineID:  ptlh.TimelineID,
			SwitchPoint: ptlh.SwitchPoint,
			Reason:      ptlh.Reason,
		})
	}
	return ntlsh
}

// PostgresTimelineHistory defines a PostgreSQL timeline
type PostgresTimelineHistory struct {
	TimelineID  uint64
	SwitchPoint uint64
	Reason      string
}

// GetTimelineHistory returns a PostgresTimelineHistory for a PostgresTimelinesHistory
func (tlsh PostgresTimelinesHistory) GetTimelineHistory(id uint64) *PostgresTimelineHistory {
	for _, tlh := range tlsh {
		if tlh.TimelineID == id {
			return tlh
		}
	}
	return nil
}

// PostgresState defines the state of a PostgreSQL instance
type PostgresState struct {
	Initialized      bool
	Role             common.Role
	SystemID         string
	TimelineID       uint64
	XLogPos          uint64
	TimelinesHistory PostgresTimelinesHistory
}

// Copy returns a shallow copy
func (p *PostgresState) Copy() *PostgresState {
	if p == nil {
		return nil
	}
	np := *p
	np.TimelinesHistory = p.TimelinesHistory.Copy()
	return &np
}

// SentinelsInfo stores all info on sentinels
type SentinelsInfo []*SentinelInfo

/*
func (s SentinelsInfo) Len() int           { return len(s) }
func (s SentinelsInfo) Less(i, j int) bool { return s[i].ID < s[j].ID }
func (s SentinelsInfo) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
*/

// SentinelInfo stores all info on a sentinel
type SentinelInfo struct {
	ID            string
	ListenAddress string
	Port          string
}

// ProxiesInfo stores all info on proxies
type ProxiesInfo []*ProxyInfo

/*
func (p ProxiesInfo) Len() int           { return len(p) }
func (p ProxiesInfo) Less(i, j int) bool { return p[i].ID < p[j].ID }
func (p ProxiesInfo) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
*/

// ProxyInfo stores all info on a proxy
type ProxyInfo struct {
	ID                 string
	ListenAddress      string
	Port               string
	ClusterViewVersion int
}
