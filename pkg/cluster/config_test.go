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
			cfg: mergeDefaults(&NilConfig{}).ToConfig(),
			err: nil,
		},
		// Test duration parsing
		{
			in:  `{ "requesttimeout": "3s" }`,
			cfg: mergeDefaults(&NilConfig{RequestTimeout: DurationP(3 * time.Second)}).ToConfig(),
			err: nil,
		},
		{
			in:  `{ "requesttimeout": "3000ms" }`,
			cfg: mergeDefaults(&NilConfig{RequestTimeout: DurationP(3 * time.Second)}).ToConfig(),
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
			cfg: mergeDefaults(&NilConfig{
				RequestTimeout:         DurationP(10 * time.Second),
				SleepInterval:          DurationP(10 * time.Second),
				KeeperFailInterval:     DurationP(100 * time.Second),
				PGReplUser:             StringP("username"),
				PGReplPassword:         StringP("password"),
				MaxStandbysPerSender:   UintP(5),
				SynchronousReplication: BoolP(true),
			}).ToConfig(),
			err: nil,
		},
	}

	for i, tt := range tests {
		var nilCfg *NilConfig
		err := json.Unmarshal([]byte(tt.in), &nilCfg)
		nilCfg.MergeDefaults()
		cfg := nilCfg.ToConfig()
		if tt.err != nil {
			if err == nil {
				t.Errorf("#%d: got no error, wanted error: %v", i, tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("#%d: got error: %v, wanted error: %v", i, err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("#%d: unexpected error: %v", i, err)
			}
			if !reflect.DeepEqual(cfg, tt.cfg) {
				t.Errorf(spew.Sprintf("#%d: wrong config: got: %#v, want: %#v", i, cfg, tt.cfg))
			}
		}

	}
}

func mergeDefaults(c *NilConfig) *NilConfig {
	c.MergeDefaults()
	return c
}
