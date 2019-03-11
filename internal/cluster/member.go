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

	"github.com/sorintlab/stolon/internal/common"

	"github.com/mitchellh/copystructure"
)

type KeepersInfo map[string]*KeeperInfo

func (k KeepersInfo) DeepCopy() KeepersInfo {
	if k == nil {
		return nil
	}
	nk, err := copystructure.Copy(k)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(k, nk) {
		panic("not equal")
	}
	return nk.(KeepersInfo)
}

type KeeperInfo struct {
	// An unique id for this info, used to know when this the keeper info
	// has been updated
	InfoUID string `json:"infoUID,omitempty"`

	UID        string `json:"uid,omitempty"`
	ClusterUID string `json:"clusterUID,omitempty"`
	BootUUID   string `json:"bootUUID,omitempty"`

	PostgresBinaryVersion PostgresBinaryVersion `json:"postgresBinaryVersion,omitempty"`

	// nil means not specified
	Priority *int `json:"priority,omitempty"`

	PostgresState *PostgresState `json:"postgresState,omitempty"`

	CanBeMaster             *bool `json:"canBeMaster,omitempty"`
	CanBeSynchronousReplica *bool `json:"canBeSynchronousReplica,omitempty"`
}

func (k *KeeperInfo) DeepCopy() *KeeperInfo {
	if k == nil {
		return nil
	}
	nk, err := copystructure.Copy(k)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(k, nk) {
		panic("not equal")
	}
	return nk.(*KeeperInfo)
}

type PostgresTimelinesHistory []*PostgresTimelineHistory

type PostgresTimelineHistory struct {
	TimelineID  uint64 `json:"timelineID,omitempty"`
	SwitchPoint uint64 `json:"switchPoint,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func (tlsh PostgresTimelinesHistory) GetTimelineHistory(id uint64) *PostgresTimelineHistory {
	for _, tlh := range tlsh {
		if tlh.TimelineID == id {
			return tlh
		}
	}
	return nil
}

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

func (p *PostgresState) DeepCopy() *PostgresState {
	if p == nil {
		return nil
	}
	np, err := copystructure.Copy(p)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(p, np) {
		panic("not equal")
	}
	return np.(*PostgresState)
}

type SentinelsInfo []*SentinelInfo

func (s SentinelsInfo) Len() int           { return len(s) }
func (s SentinelsInfo) Less(i, j int) bool { return s[i].UID < s[j].UID }
func (s SentinelsInfo) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type SentinelInfo struct {
	UID string
}

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

type ProxiesInfo map[string]*ProxyInfo

func (p ProxiesInfo) DeepCopy() ProxiesInfo {
	if p == nil {
		return nil
	}
	np, err := copystructure.Copy(p)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(p, np) {
		panic("not equal")
	}
	return np.(ProxiesInfo)
}

func (p ProxiesInfo) ToSlice() ProxiesInfoSlice {
	pis := ProxiesInfoSlice{}
	for _, pi := range p {
		pis = append(pis, pi)
	}
	return pis
}

type ProxiesInfoSlice []*ProxyInfo

func (p ProxiesInfoSlice) Len() int           { return len(p) }
func (p ProxiesInfoSlice) Less(i, j int) bool { return p[i].UID < p[j].UID }
func (p ProxiesInfoSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
