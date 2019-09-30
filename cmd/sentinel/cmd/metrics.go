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
)

var (
	lastCheckSuccessSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stolon_sentinel_last_cluster_check_success_seconds",
			Help: "Last time we successfully performed a cluster check as seconds since unix epoch",
		},
	)
	// These metrics will be provided by the collector. They represent values
	// that shouldn't be updated as part of the sentinel code, but should be
	// gathered just prior to providing Prometheus with a measurement.
	isLeaderDesc = prometheus.NewDesc(
		"stolon_sentinel_is_leader",
		"Set to 1 if the sentinel is currently a leader",
		[]string{},
		nil,
	)
	leaderCountDesc = prometheus.NewDesc(
		"stolon_sentinel_leader_count",
		"Number of times this sentinel has been elected as leader",
		[]string{},
		nil,
	)
)

// Register the static methods on the default Prometheus registry automatically
func init() {
	prometheus.MustRegister(lastCheckSuccessSeconds)
}

func mustRegisterSentinelCollector(s *Sentinel) {
	prometheus.MustRegister(
		sentinelCollector{s},
	)
}

type sentinelCollector struct {
	*Sentinel
}

func (c sentinelCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c sentinelCollector) Collect(ch chan<- prometheus.Metric) {
	var isLeaderValue float64
	isLeader, leaderCount := c.Sentinel.leaderInfo()
	if isLeader {
		isLeaderValue = 1
	}

	ch <- prometheus.MustNewConstMetric(isLeaderDesc, prometheus.GaugeValue, isLeaderValue)
	ch <- prometheus.MustNewConstMetric(leaderCountDesc, prometheus.GaugeValue, float64(leaderCount))
}
