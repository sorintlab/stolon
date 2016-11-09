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

package v0

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
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
			in:  `{ "request_timeout": "3s" }`,
			cfg: mergeDefaults(&NilConfig{RequestTimeout: &Duration{3 * time.Second}}).ToConfig(),
			err: nil,
		},
		{
			in:  `{ "request_timeout": "3000ms" }`,
			cfg: mergeDefaults(&NilConfig{RequestTimeout: &Duration{3 * time.Second}}).ToConfig(),
			err: nil,
		},
		{
			in:  `{ "request_timeout": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: request_timeout must be positive"),
		},
		{
			in:  `{ "request_timeout": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: request_timeout must be positive"),
		},
		{
			in:  `{ "sleep_interval": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: sleep_interval must be positive"),
		},
		{
			in:  `{ "keeper_fail_interval": "-3s" }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: keeper_fail_interval must be positive"),
		},
		{
			in:  `{ "max_standbys_per_sender": 0 }`,
			cfg: nil,
			err: fmt.Errorf("config validation failed: max_standbys_per_sender must be at least 1"),
		},
		// All options defined
		{
			in: `{ "request_timeout": "10s", "sleep_interval": "10s", "keeper_fail_interval": "100s", "max_standbys_per_sender": 5, "synchronous_replication": true, "init_with_multiple_keepers": true,
			       "pg_parameters": {
			         "param01": "value01"
				}
			     }`,
			cfg: mergeDefaults(&NilConfig{
				RequestTimeout:          &Duration{10 * time.Second},
				SleepInterval:           &Duration{10 * time.Second},
				KeeperFailInterval:      &Duration{100 * time.Second},
				MaxStandbysPerSender:    UintP(5),
				SynchronousReplication:  BoolP(true),
				InitWithMultipleKeepers: BoolP(true),
				PGParameters: &map[string]string{
					"param01": "value01",
				},
			}).ToConfig(),
			err: nil,
		},
	}

	for i, tt := range tests {
		var nilCfg *NilConfig
		err := json.Unmarshal([]byte(tt.in), &nilCfg)
		if err != nil {
			if tt.err == nil {
				t.Errorf("#%d: unexpected error: %v", i, err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("#%d: got error: %v, wanted error: %v", i, err, tt.err)
			}
		} else {
			nilCfg.MergeDefaults()
			cfg := nilCfg.ToConfig()
			if tt.err != nil {
				t.Errorf("#%d: got no error, wanted error: %v", i, tt.err)
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

func TestNilConfigCopy(t *testing.T) {
	// cfg and origCfg are declared in an identical way. It's not
	// possible to take a shallow copy since cfg must absolutely
	// not change as it's used for the reflect.DeepEqual comparison.
	cfg := mergeDefaults(&NilConfig{
		RequestTimeout:          &Duration{10 * time.Second},
		SleepInterval:           &Duration{10 * time.Second},
		KeeperFailInterval:      &Duration{10 * time.Second},
		MaxStandbysPerSender:    UintP(5),
		SynchronousReplication:  BoolP(true),
		InitWithMultipleKeepers: BoolP(true),
		PGParameters: &map[string]string{
			"param01": "value01",
		},
	})
	origCfg := mergeDefaults(&NilConfig{
		RequestTimeout:          &Duration{10 * time.Second},
		SleepInterval:           &Duration{10 * time.Second},
		KeeperFailInterval:      &Duration{10 * time.Second},
		MaxStandbysPerSender:    UintP(5),
		SynchronousReplication:  BoolP(true),
		InitWithMultipleKeepers: BoolP(true),
		PGParameters: &map[string]string{
			"param01": "value01",
		},
	})

	// Now take a origCfg copy, change all its fields and check that origCfg isn't changed
	newCfg := origCfg.Copy()
	newCfg.RequestTimeout = &Duration{20 * time.Second}
	newCfg.SleepInterval = &Duration{20 * time.Second}
	newCfg.KeeperFailInterval = &Duration{20 * time.Second}
	newCfg.MaxStandbysPerSender = UintP(10)
	newCfg.SynchronousReplication = BoolP(false)
	newCfg.InitWithMultipleKeepers = BoolP(false)
	(*newCfg.PGParameters)["param01"] = "anothervalue01"

	if !reflect.DeepEqual(origCfg, cfg) {
		t.Errorf("Original config shouldn't be changed")
	}

}

func TestConfigCopy(t *testing.T) {
	// cfg and origCfg are declared in an identical way. It's not
	// possible to take a shallow copy since cfg must absolutely
	// not change as it's used for the reflect.DeepEqual comparison.
	cfg := mergeDefaults(&NilConfig{
		RequestTimeout:          &Duration{10 * time.Second},
		SleepInterval:           &Duration{10 * time.Second},
		KeeperFailInterval:      &Duration{100 * time.Second},
		MaxStandbysPerSender:    UintP(5),
		SynchronousReplication:  BoolP(true),
		InitWithMultipleKeepers: BoolP(true),
		PGParameters: &map[string]string{
			"param01": "value01",
		},
	}).ToConfig()
	origCfg := mergeDefaults(&NilConfig{
		RequestTimeout:          &Duration{10 * time.Second},
		SleepInterval:           &Duration{10 * time.Second},
		KeeperFailInterval:      &Duration{100 * time.Second},
		MaxStandbysPerSender:    UintP(5),
		SynchronousReplication:  BoolP(true),
		InitWithMultipleKeepers: BoolP(true),
		PGParameters: &map[string]string{
			"param01": "value01",
		},
	}).ToConfig()

	// Now take a origCfg copy, change all its fields and check that origCfg isn't changed
	newCfg := origCfg.Copy()
	newCfg.RequestTimeout = 20 * time.Second
	newCfg.SleepInterval = 20 * time.Second
	newCfg.KeeperFailInterval = 20 * time.Second
	newCfg.MaxStandbysPerSender = 10
	newCfg.SynchronousReplication = false
	newCfg.InitWithMultipleKeepers = false
	newCfg.PGParameters["param01"] = "anothervalue01"

	if !reflect.DeepEqual(origCfg, cfg) {
		t.Errorf("Original config shouldn't be changed")
	}

}
