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
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/davecgh/go-spew/spew"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		in  string
		cfg *Config
		err error
	}{
		{
			in:  "{}",
			cfg: &DefaultConfig,
			err: nil,
		},
		// Test duration parsing
		{
			in:  `{ "leasettl": "3s" }`,
			cfg: mergeDefaultConfig(&Config{LeaseTTL: 3 * time.Second}),
			err: nil,
		},
		{
			in:  `{ "leasettl": "3000ms" }`,
			cfg: mergeDefaultConfig(&Config{LeaseTTL: 3 * time.Second}),
			err: nil,
		},
		{
			in:  `{ "leasettl": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("LeaseTTL must be positive"),
		},
		{
			in:  `{ "requesttimeout": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("RequestTimeout must be positive"),
		},
		{
			in:  `{ "sleepinterval": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("SleepInterval must be positive"),
		},
		{
			in:  `{ "memberfailinterval": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("MemberFailInterval must be positive"),
		},
		{
			in:  `{ "pgrepluser": "" }`,
			cfg: nil,
			err: fmt.Errorf("PGReplUser cannot be empty"),
		},
		{
			in:  `{ "pgreplpassword": "" }`,
			cfg: nil,
			err: fmt.Errorf("PGReplPassword cannot be empty"),
		},
		{
			in:  `{ "maxstandbyspersender": 0 }`,
			cfg: nil,
			err: fmt.Errorf("MaxStandbysPerSender must greater than 0"),
		},
		// All options defined
		{
			in: `{ "leasettl": "60s", "requestTimeout": "10s", "sleepInterval": "10s", "memberFailInterval": "100s", "pgrepluser": "username", "pgreplpassword": "password", "maxstandbyspersender": 5 }`,
			cfg: mergeDefaultConfig(&Config{
				LeaseTTL:             60 * time.Second,
				RequestTimeout:       10 * time.Second,
				SleepInterval:        10 * time.Second,
				MemberFailInterval:   100 * time.Second,
				PGReplUser:           "username",
				PGReplPassword:       "password",
				MaxStandbysPerSender: 5,
			}),
			err: nil,
		},
	}

	for i, tt := range tests {
		cfg, err := ParseConfig([]byte(tt.in))
		if tt.err != nil {
			if err == nil {
				t.Errorf("got no error, wanted error: %v", tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("got error: %v, wanted error: %v", err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(cfg, tt.cfg) {
				t.Errorf(spew.Sprintf("#%d: wrong config: got: %#v, want: %#v", i, cfg, tt.cfg))
			}
		}

	}
}

func mergeDefaultConfig(ic *Config) *Config {
	c := DefaultConfig
	if ic.LeaseTTL != 0 {
		c.LeaseTTL = ic.LeaseTTL
	}
	if ic.RequestTimeout != 0 {
		c.RequestTimeout = ic.RequestTimeout
	}
	if ic.SleepInterval != 0 {
		c.SleepInterval = ic.SleepInterval
	}
	if ic.MemberFailInterval != 0 {
		c.MemberFailInterval = ic.MemberFailInterval
	}
	if ic.PGReplUser != "" {
		c.PGReplUser = ic.PGReplUser
	}
	if ic.PGReplPassword != "" {
		c.PGReplPassword = ic.PGReplPassword
	}
	if ic.MaxStandbysPerSender != 0 {
		c.MaxStandbysPerSender = ic.MaxStandbysPerSender
	}
	return &c
}
