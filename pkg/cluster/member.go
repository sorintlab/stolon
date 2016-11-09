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

type KeepersInfo map[string]*KeeperInfo

type KeeperInfo struct {
	UID           string
	Generation    int64
	ListenAddress string
	Port          string
}

func (k *KeeperInfo) Copy() *KeeperInfo {
	if k == nil {
		return nil
	}
	nk := *k
	return &nk
}

type PostgresTimelinesHistory []*PostgresTimelineHistory

func (tlsh PostgresTimelinesHistory) Copy() PostgresTimelinesHistory {
	if tlsh == nil {
		return nil
	}
	ntlsh := make(PostgresTimelinesHistory, len(tlsh))
	copy(ntlsh, tlsh)
	return ntlsh
}

type PostgresTimelineHistory struct {
	TimelineID  uint64
	SwitchPoint uint64
	Reason      string
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
	UID        string
	Generation int64

	ListenAddress string `json:"listenAddress,omitempty"`
	Port          string `json:"port,omitempty"`

	Healthy          bool
	SystemID         string
	TimelineID       uint64
	XLogPos          uint64
	TimelinesHistory PostgresTimelinesHistory
}

func (p *PostgresState) Copy() *PostgresState {
	if p == nil {
		return nil
	}
	np := *p
	np.TimelinesHistory = p.TimelinesHistory.Copy()
	return &np
}

type KeepersDiscoveryInfo []*KeeperDiscoveryInfo

type KeeperDiscoveryInfo struct {
	ListenAddress string
	Port          string
}

type SentinelsInfo []*SentinelInfo

func (s SentinelsInfo) Len() int           { return len(s) }
func (s SentinelsInfo) Less(i, j int) bool { return s[i].UID < s[j].UID }
func (s SentinelsInfo) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type SentinelInfo struct {
	UID string
}

type ProxiesInfo []*ProxyInfo

func (p ProxiesInfo) Len() int           { return len(p) }
func (p ProxiesInfo) Less(i, j int) bool { return p[i].UID < p[j].UID }
func (p ProxiesInfo) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type ProxyInfo struct {
	UID             string
	ListenAddress   string
	Port            string
	ProxyUID        string
	ProxyGeneration int64
}
