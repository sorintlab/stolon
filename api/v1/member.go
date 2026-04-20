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

package v1

import (
	"time"

	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/util"
)

// PostgresTimelinesHistory stores all PostgreSQL timelines belonging to this cluster
type PostgresTimelinesHistory []*PostgresTimelineHistory

// PostgresTimelineHistory defines a PostgreSQL timeline
type PostgresTimelineHistory struct {
	TimelineID  uint64 `json:"timelineID,omitempty"`
	SwitchPoint uint64 `json:"switchPoint,omitempty"`
	Reason      string `json:"reason,omitempty"`
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
	UID        string `json:"uid,omitempty"`
	Generation int64  `json:"generation,omitempty"`

	ListenAddress string `json:"listenAddress,omitempty"`
	Port          string `json:"port,omitempty"`

	Healthy bool `json:"healthy,omitempty"`

	SystemID         string                   `json:"systemID,omitempty"`
	TimelineID       uint64                   `json:"timelineID,omitempty"`
	XLogPos          uint64                   `json:"xLogPos,omitempty"`
	TimelinesHistory PostgresTimelinesHistory `json:"timelinesHistory,omitempty"`

	PGParameters        common.Parameters `json:"pgParameters,omitempty"`
	SynchronousStandbys []string          `json:"synchronousStandbys"`
	OlderWalFile        string            `json:"olderWalFile,omitempty"`
}

// DeepCopy returns a copy of the PostgresState resource
func (p *PostgresState) DeepCopy() (dc *PostgresState) {
	return util.DeepCopy(p)
}

// SentinelsInfo stores all SentinelInfo resources for a cluster
type SentinelsInfo []*SentinelInfo

func (s SentinelsInfo) Len() int           { return len(s) }
func (s SentinelsInfo) Less(i, j int) bool { return s[i].UID < s[j].UID }
func (s SentinelsInfo) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// SentinelInfo stores all info for a sentinel
type SentinelInfo struct {
	UID string
}

// ProxyInfo stores all info for a proxy
type ProxyInfo struct {
	// An unique id for this info, used to know when the proxy info
	// has been updated
	InfoUID string `json:"infoUID,omitempty"`

	UID        string
	Generation int64

	// ProxyTimeout is the current proxyTimeout used by the proxy
	// at the time of publishing its state.
	// It's used by the sentinel to know for how much time the
	// proxy should be considered active.
	ProxyTimeout time.Duration
}

// ProxiesInfo stores inbfo about all Proxies for this cluster
type ProxiesInfo map[string]*ProxyInfo

// DeepCopy returns a copy of the ProxiesInfo resource
func (p *ProxiesInfo) DeepCopy() (dc *ProxiesInfo) {
	return util.DeepCopy(p)
}

// ToSlice converts the ProxiesInfo map into a slice of ProxyInfo resources
func (p ProxiesInfo) ToSlice() ProxiesInfoSlice {
	pis := ProxiesInfoSlice{}
	for _, pi := range p {
		pis = append(pis, pi)
	}
	return pis
}

// ProxiesInfoSlice defines a slice of ProxyInfo resources
type ProxiesInfoSlice []*ProxyInfo

func (p ProxiesInfoSlice) Len() int           { return len(p) }
func (p ProxiesInfoSlice) Less(i, j int) bool { return p[i].UID < p[j].UID }
func (p ProxiesInfoSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
