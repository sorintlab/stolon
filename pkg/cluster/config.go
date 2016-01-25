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
	"strings"
	"time"
)

const (
	DefaultProxyCheckInterval = 5 * time.Second

	DefaultRequestTimeout         = 10 * time.Second
	DefaultSleepInterval          = 5 * time.Second
	DefaultKeeperFailInterval     = 20 * time.Second
	DefaultPGReplUser             = "repluser"
	DefaultPGReplPassword         = "replpassword"
	DefaultMaxStandbysPerSender   = 3
	DefaultSynchronousReplication = false
)

type NilConfig struct {
	RequestTimeout         *Duration          `json:"request_timeout,omitempty"`
	SleepInterval          *Duration          `json:"sleep_interval,omitempty"`
	KeeperFailInterval     *Duration          `json:"keeper_fail_interval,omitempty"`
	PGReplUser             *string            `json:"pg_repl_user,omitempty"`
	PGReplPassword         *string            `json:"pg_repl_password,omitempty"`
	MaxStandbysPerSender   *uint              `json:"max_standbys_per_sender,omitempty"`
	SynchronousReplication *bool              `json:"synchronous_replication,omitempty"`
	PGParameters           *map[string]string `json:"pg_parameters,omitempty"`
}

type Config struct {
	// Time after which any request (keepers checks from sentinel etc...) will fail.
	RequestTimeout time.Duration
	// Interval to wait before next check (for every component: keeper, sentinel, proxy).
	SleepInterval time.Duration
	// Interval after the first fail to declare a keeper as not healthy.
	KeeperFailInterval time.Duration
	// PostgreSQL replication username
	PGReplUser string
	// PostgreSQL replication password
	PGReplPassword string
	// Max number of standbys for every sender. A sender can be a master or
	// another standby (with cascading replication).
	MaxStandbysPerSender uint
	// Use Synchronous replication between master and its standbys
	SynchronousReplication bool
	// Map of postgres parameters
	PGParameters map[string]string
}

func StringP(s string) *string {
	return &s
}

func UintP(u uint) *uint {
	return &u
}

func BoolP(b bool) *bool {
	return &b
}

func DurationP(d Duration) *Duration {
	return &d
}

func MapStringP(m map[string]string) *map[string]string {
	nm := map[string]string{}
	for k, v := range m {
		nm[k] = v
	}
	return &nm
}

type nilConfig NilConfig

func (c *NilConfig) UnmarshalJSON(in []byte) error {
	var nc nilConfig
	if err := json.Unmarshal(in, &nc); err != nil {
		return err
	}
	*c = NilConfig(nc)
	if err := c.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %v", err)
	}
	return nil
}

func (c *NilConfig) Copy() *NilConfig {
	if c == nil {
		return c
	}
	var nc NilConfig
	if c.RequestTimeout != nil {
		nc.RequestTimeout = DurationP(*c.RequestTimeout)
	}
	if c.SleepInterval != nil {
		nc.SleepInterval = DurationP(*c.SleepInterval)
	}
	if c.KeeperFailInterval != nil {
		nc.KeeperFailInterval = DurationP(*c.KeeperFailInterval)
	}
	if c.PGReplUser != nil {
		nc.PGReplUser = StringP(*c.PGReplUser)
	}
	if c.PGReplPassword != nil {
		nc.PGReplPassword = StringP(*c.PGReplPassword)
	}
	if c.MaxStandbysPerSender != nil {
		nc.MaxStandbysPerSender = UintP(*c.MaxStandbysPerSender)
	}
	if c.SynchronousReplication != nil {
		nc.SynchronousReplication = BoolP(*c.SynchronousReplication)
	}
	if c.PGParameters != nil {
		nc.PGParameters = MapStringP(*c.PGParameters)
	}
	return &nc
}

func (c *Config) Copy() *Config {
	if c == nil {
		return c
	}
	nc := *c
	return &nc
}

// Duration is needed to be able to marshal/unmarshal json strings with time
// unit (eg. 3s, 100ms) instead of ugly times in nanoseconds.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	du, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = du
	return nil
}

func (c *NilConfig) Validate() error {
	if c.RequestTimeout != nil && (*c.RequestTimeout).Duration < 0 {
		return fmt.Errorf("request_timeout must be positive")
	}
	if c.SleepInterval != nil && (*c.SleepInterval).Duration < 0 {
		return fmt.Errorf("sleep_interval must be positive")
	}
	if c.KeeperFailInterval != nil && (*c.KeeperFailInterval).Duration < 0 {
		return fmt.Errorf("keeper_fail_interval must be positive")
	}
	if c.PGReplUser != nil && *c.PGReplUser == "" {
		return fmt.Errorf("pg_repl_user cannot be empty")
	}
	if c.PGReplPassword != nil && *c.PGReplPassword == "" {
		return fmt.Errorf("pg_repl_password cannot be empty")
	}
	if c.MaxStandbysPerSender != nil && *c.MaxStandbysPerSender < 1 {
		return fmt.Errorf("max_standbys_per_sender must be at least 1")
	}
	return nil
}

func (c *NilConfig) MergeDefaults() {
	if c.RequestTimeout == nil {
		c.RequestTimeout = &Duration{DefaultRequestTimeout}
	}
	if c.SleepInterval == nil {
		c.SleepInterval = &Duration{DefaultSleepInterval}
	}
	if c.KeeperFailInterval == nil {
		c.KeeperFailInterval = &Duration{DefaultKeeperFailInterval}
	}
	if c.PGReplUser == nil {
		c.PGReplUser = StringP(DefaultPGReplUser)
	}
	if c.PGReplPassword == nil {
		c.PGReplPassword = StringP(DefaultPGReplPassword)
	}
	if c.MaxStandbysPerSender == nil {
		c.MaxStandbysPerSender = UintP(DefaultMaxStandbysPerSender)
	}
	if c.SynchronousReplication == nil {
		c.SynchronousReplication = BoolP(DefaultSynchronousReplication)
	}
	if c.PGParameters == nil {
		c.PGParameters = &map[string]string{}
	}
}

func (c *NilConfig) ToConfig() *Config {
	nc := c.Copy()
	nc.MergeDefaults()
	return &Config{
		RequestTimeout:         (*nc.RequestTimeout).Duration,
		SleepInterval:          (*nc.SleepInterval).Duration,
		KeeperFailInterval:     (*nc.KeeperFailInterval).Duration,
		PGReplUser:             *nc.PGReplUser,
		PGReplPassword:         *nc.PGReplPassword,
		MaxStandbysPerSender:   *nc.MaxStandbysPerSender,
		SynchronousReplication: *nc.SynchronousReplication,
		PGParameters:           *nc.PGParameters,
	}
}

func NewDefaultConfig() *Config {
	nc := &NilConfig{}
	nc.MergeDefaults()
	return nc.ToConfig()
}
