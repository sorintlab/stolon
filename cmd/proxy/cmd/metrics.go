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
)

func init() {
	prometheus.MustRegister(proxyHealthGauge)
}
