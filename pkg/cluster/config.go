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

// jsonConfig is a copy of Config with all the time.Duration types converted to duration.
type jsonConfig struct {
	RequestTimeout         *duration `json:",omitempty"`
	SleepInterval          *duration `json:",omitempty"`
	KeeperFailInterval     *duration `json:",omitempty"`
	PGReplUser             *string   `json:",omitempty"`
	PGReplPassword         *string   `json:",omitempty"`
	MaxStandbysPerSender   *uint     `json:",omitempty"`
	SynchronousReplication *bool     `json:",omitempty"`
}

type NilConfig struct {
	RequestTimeout         *time.Duration `json:",omitempty"`
	SleepInterval          *time.Duration `json:",omitempty"`
	KeeperFailInterval     *time.Duration `json:",omitempty"`
	PGReplUser             *string        `json:",omitempty"`
	PGReplPassword         *string        `json:",omitempty"`
	MaxStandbysPerSender   *uint          `json:",omitempty"`
	SynchronousReplication *bool          `json:",omitempty"`
}

type Config struct {
	// Time after which any request (to etcd, keepers checks from sentinel etc...) will fail.
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

func DurationP(d time.Duration) *time.Duration {
	return &d
}

func (c *NilConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.ToJsonConfig())
}

func (c *NilConfig) UnmarshalJSON(in []byte) error {
	var jc jsonConfig
	err := json.Unmarshal(in, &jc)
	if err != nil {
		return err
	}
	*c = *jc.ToConfig()
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
	return &nc
}

func (c *Config) Copy() *Config {
	if c == nil {
		return c
	}
	nc := *c
	return &nc
}

func (c *NilConfig) ToJsonConfig() *jsonConfig {
	return &jsonConfig{
		RequestTimeout:         (*duration)(c.RequestTimeout),
		SleepInterval:          (*duration)(c.SleepInterval),
		KeeperFailInterval:     (*duration)(c.KeeperFailInterval),
		PGReplUser:             c.PGReplUser,
		PGReplPassword:         c.PGReplPassword,
		MaxStandbysPerSender:   c.MaxStandbysPerSender,
		SynchronousReplication: c.SynchronousReplication,
	}
}

func (jc *jsonConfig) ToConfig() *NilConfig {
	return &NilConfig{
		RequestTimeout:         (*time.Duration)(jc.RequestTimeout),
		SleepInterval:          (*time.Duration)(jc.SleepInterval),
		KeeperFailInterval:     (*time.Duration)(jc.KeeperFailInterval),
		PGReplUser:             jc.PGReplUser,
		PGReplPassword:         jc.PGReplPassword,
		MaxStandbysPerSender:   jc.MaxStandbysPerSender,
		SynchronousReplication: jc.SynchronousReplication,
	}
}

// duration is needed to be able to marshal/unmarshal json strings with time
// unit (eg. 3s, 100ms) instead of ugly times in nanoseconds.
type duration time.Duration

func (d duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *duration) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	du, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = duration(du)
	return nil
}

func (c *NilConfig) Validate() error {
	if c.RequestTimeout != nil && *c.RequestTimeout < 0 {
		return fmt.Errorf("RequestTimeout must be positive")
	}
	if c.SleepInterval != nil && *c.SleepInterval < 0 {
		return fmt.Errorf("SleepInterval must be positive")
	}
	if c.KeeperFailInterval != nil && *c.KeeperFailInterval < 0 {
		return fmt.Errorf("KeeperFailInterval must be positive")
	}
	if c.PGReplUser != nil && *c.PGReplUser == "" {
		return fmt.Errorf("PGReplUser cannot be empty")
	}
	if c.PGReplPassword != nil && *c.PGReplPassword == "" {
		return fmt.Errorf("PGReplPassword cannot be empty")
	}
	if c.MaxStandbysPerSender != nil && *c.MaxStandbysPerSender < 1 {
		return fmt.Errorf("MaxStandbysPerSender must be at least 1")
	}
	return nil
}

func (c *NilConfig) MergeDefaults() {
	if c.RequestTimeout == nil {
		c.RequestTimeout = DurationP(DefaultRequestTimeout)
	}
	if c.SleepInterval == nil {
		c.SleepInterval = DurationP(DefaultSleepInterval)
	}
	if c.KeeperFailInterval == nil {
		c.KeeperFailInterval = DurationP(DefaultKeeperFailInterval)
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
}

func (c *NilConfig) ToConfig() *Config {
	nc := c.Copy()
	nc.MergeDefaults()
	return &Config{
		RequestTimeout:         *nc.RequestTimeout,
		SleepInterval:          *nc.SleepInterval,
		KeeperFailInterval:     *nc.KeeperFailInterval,
		PGReplUser:             *nc.PGReplUser,
		PGReplPassword:         *nc.PGReplPassword,
		MaxStandbysPerSender:   *nc.MaxStandbysPerSender,
		SynchronousReplication: *nc.SynchronousReplication,
	}
}

func NewDefaultConfig() *Config {
	nc := &NilConfig{}
	nc.MergeDefaults()
	return nc.ToConfig()
}
