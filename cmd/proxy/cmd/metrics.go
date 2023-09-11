// Copyright 2021 Sorint.lab
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
)

var (
	proxyHealthGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_proxy_health",
			Help: "Set to 1 if proxy healthy and accepting connections",
		},
	)

	clusterdataLastValidUpdateSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_proxy_clusterdata_last_valid_update_seconds",
			Help: "Last time we received a valid clusterdata from our store as seconds since unix epoch",
		},
	)

	proxyListenerStartedSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_proxy_listener_started_seconds",
			Help: "Last time we started the proxy listener as seconds since unix epoch",
		},
	)

	getClusterInfoErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "stolon_proxy_get_cluster_info_errors",
			Help: "Count of failed getting and parsing cluster info operationss",
		},
	)

	updateProxyInfoErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "stolon_proxy_update_proxy_info_errors",
			Help: "Count of update proxyInfo failures",
		},
	)
)

func init() {
	prometheus.MustRegister(proxyHealthGauge)
	prometheus.MustRegister(clusterdataLastValidUpdateSeconds)
	prometheus.MustRegister(proxyListenerStartedSeconds)
	prometheus.MustRegister(getClusterInfoErrors)
	prometheus.MustRegister(updateProxyInfoErrors)
}
