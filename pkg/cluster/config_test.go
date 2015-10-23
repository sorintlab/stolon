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
	"encoding/json"
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
			in:  `{ "requesttimeout": "3s" }`,
			cfg: mergeDefaultConfig(&Config{RequestTimeout: 3 * time.Second}),
			err: nil,
		},
		{
			in:  `{ "requesttimeout": "3000ms" }`,
			cfg: mergeDefaultConfig(&Config{RequestTimeout: 3 * time.Second}),
			err: nil,
		},
		{
			in:  `{ "requesttimeout": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: RequestTimeout must be positive"),
		},
		{
			in:  `{ "requesttimeout": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: RequestTimeout must be positive"),
		},
		{
			in:  `{ "sleepinterval": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: SleepInterval must be positive"),
		},
		{
			in:  `{ "keeperfailinterval": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: KeeperFailInterval must be positive"),
		},
		{
			in:  `{ "pgrepluser": "" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: PGReplUser cannot be empty"),
		},
		{
			in:  `{ "pgreplpassword": "" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: PGReplPassword cannot be empty"),
		},
		{
			in:  `{ "maxstandbyspersender": 0 }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: MaxStandbysPerSender must be at least 1"),
		},
		// All options defined
		{
			in: `{ "requestTimeout": "10s", "sleepInterval": "10s", "keeperFailInterval": "100s", "pgrepluser": "username", "pgreplpassword": "password", "maxstandbyspersender": 5, "synchronousreplication": true}`,
			cfg: mergeDefaultConfig(&Config{
				RequestTimeout:         10 * time.Second,
				SleepInterval:          10 * time.Second,
				KeeperFailInterval:     100 * time.Second,
				PGReplUser:             "username",
				PGReplPassword:         "password",
				MaxStandbysPerSender:   5,
				SynchronousReplication: true,
			}),
			err: nil,
		},
	}

	for i, tt := range tests {
		var cfg *Config
		err := json.Unmarshal([]byte(tt.in), &cfg)
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
	if ic.RequestTimeout != 0 {
		c.RequestTimeout = ic.RequestTimeout
	}
	if ic.SleepInterval != 0 {
		c.SleepInterval = ic.SleepInterval
	}
	if ic.KeeperFailInterval != 0 {
		c.KeeperFailInterval = ic.KeeperFailInterval
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
	if ic.SynchronousReplication != false {
		c.SynchronousReplication = ic.SynchronousReplication
	}
	return &c
}
