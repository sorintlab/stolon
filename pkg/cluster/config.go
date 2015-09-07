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

import "time"

const (
	DefaultLeaseTTL           = 30 * time.Second
	DefaultRequestTimeout     = 10 * time.Second
	DefaultEtcdRequestTimeout = DefaultRequestTimeout
	DefaultCheckInterval      = 5 * time.Second
	DefaultMemberFailInterval = 4 * DefaultCheckInterval
	DefaultPGReplUser         = "repluser"
	DefaultPGReplPassword     = "replpassword"
	DefaultMaxSlaves          = 3
)

type Config struct {
	LeaseTTL           time.Duration
	RequestTimeout     time.Duration
	EtcdRequestTimeout time.Duration
	CheckInterval      time.Duration
	MemberFailInterval time.Duration
	PGReplUser         string
	PGReplPassword     string
	MaxSlaves          uint
}

func NewDefaultConfig() *Config {
	return &Config{
		LeaseTTL:           DefaultLeaseTTL,
		RequestTimeout:     DefaultRequestTimeout,
		EtcdRequestTimeout: DefaultEtcdRequestTimeout,
		CheckInterval:      DefaultCheckInterval,
		MemberFailInterval: DefaultMemberFailInterval,
		PGReplUser:         DefaultPGReplUser,
		PGReplPassword:     DefaultPGReplPassword,
		MaxSlaves:          DefaultMaxSlaves,
	}

}
