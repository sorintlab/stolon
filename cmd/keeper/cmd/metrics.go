// Copyright 2019 Sorint.lab
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

package cmd

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sorintlab/stolon/internal/common"
)

var (
	clusterdataLastValidUpdateSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_keeper_clusterdata_last_valid_update_seconds",
			Help: "Last time we received a valid clusterdata from our store as seconds since unix epoch",
		},
	)
	targetRoleGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stolon_keeper_target_role",
			Help: "Keeper last requested target role",
		},
		[]string{"role"},
	)
	localRoleGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stolon_keeper_local_role",
			Help: "Keeper current local role",
		},
		[]string{"role"},
	)
	needsReloadGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_keeper_needs_reload",
			Help: "Set to 1 if Postgres requires reload",
		},
	)
	needsRestartGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_keeper_needs_restart",
			Help: "Set to 1 if Postgres requires restart",
		},
	)
	lastSyncSuccessSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_keeper_last_sync_success_seconds",
			Help: "Last time we successfully synced our keeper",
		},
	)
	sleepInterval = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_keeper_sleep_interval",
			Help: "Seconds to sleep between sync loops",
		},
	)
	shutdownSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_keeper_shutdown_seconds",
			Help: "Shutdown time (received termination signal) since unix epoch in seconds",
		},
	)
)

// setRole is a helper that controls the targetRole metric by setting only one of the
// possible roles to 1 at any one time.
func setRole(rg *prometheus.GaugeVec, role *common.Role) {
	for _, role := range common.Roles {
		rg.WithLabelValues(string(role)).Set(0)
	}

	if role != nil {
		rg.WithLabelValues(string(*role)).Set(1)
	}
}

func init() {
	prometheus.MustRegister(clusterdataLastValidUpdateSeconds)
	prometheus.MustRegister(targetRoleGauge)
	setRole(targetRoleGauge, nil)
	prometheus.MustRegister(localRoleGauge)
	setRole(localRoleGauge, nil)
	prometheus.MustRegister(needsReloadGauge)
	prometheus.MustRegister(needsRestartGauge)
	prometheus.MustRegister(lastSyncSuccessSeconds)
	prometheus.MustRegister(sleepInterval)
	prometheus.MustRegister(shutdownSeconds)
}
