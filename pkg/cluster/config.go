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
)

var (
	DefaultConfig = Config{
		RequestTimeout:         10 * time.Second,
		SleepInterval:          5 * time.Second,
		MemberFailInterval:     20 * time.Second,
		PGReplUser:             "repluser",
		PGReplPassword:         "replpassword",
		MaxStandbysPerSender:   3,
		SynchronousReplication: false,
	}
)

// jsonConfig is a copy of Config with all the time.Duration types converted to duration.
type jsonConfig struct {
	RequestTimeout         duration `json:",omitempty"`
	SleepInterval          duration `json:",omitempty"`
	MemberFailInterval     duration `json:",omitempty"`
	PGReplUser             string   `json:",omitempty"`
	PGReplPassword         string   `json:",omitempty"`
	MaxStandbysPerSender   uint     `json:",omitempty"`
	SynchronousReplication bool     `json:",omitempty"`
}

type Config struct {
	// Time after which any request (to etcd, keepers checks from sentinel etc...) will fail.
	RequestTimeout time.Duration
	// Interval to wait before next check (for every component: keeper, sentinel, proxy).
	SleepInterval time.Duration
	// Interval after the first fail to declare a member as not healthy.
	MemberFailInterval time.Duration
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

func (c *Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(configToJsonConfig(c))
}

func configToJsonConfig(c *Config) *jsonConfig {
	return &jsonConfig{
		RequestTimeout:         duration(c.RequestTimeout),
		SleepInterval:          duration(c.SleepInterval),
		MemberFailInterval:     duration(c.MemberFailInterval),
		PGReplUser:             c.PGReplUser,
		PGReplPassword:         c.PGReplPassword,
		MaxStandbysPerSender:   c.MaxStandbysPerSender,
		SynchronousReplication: c.SynchronousReplication,
	}
}

func jsonConfigToConfig(c *jsonConfig) *Config {
	return &Config{
		RequestTimeout:         time.Duration(c.RequestTimeout),
		SleepInterval:          time.Duration(c.SleepInterval),
		MemberFailInterval:     time.Duration(c.MemberFailInterval),
		PGReplUser:             c.PGReplUser,
		PGReplPassword:         c.PGReplPassword,
		MaxStandbysPerSender:   c.MaxStandbysPerSender,
		SynchronousReplication: c.SynchronousReplication,
	}
}

// duration is needed to be able to marshal/unmarshal json strings with time
// unit (eg. 3s, 100ms) instead of ugly times in nanoseconds.
type duration time.Duration

func (d duration) MarshalJSON() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
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

func NewDefaultConfig() *Config {
	c := DefaultConfig
	return &c
}

func ParseConfig(jcfg []byte) (*Config, error) {
	jc := configToJsonConfig(NewDefaultConfig())
	err := json.Unmarshal(jcfg, &jc)
	if err != nil {
		return nil, err
	}
	c := jsonConfigToConfig(jc)
	if err := ValidateConfig(c); err != nil {
		return nil, fmt.Errorf("config validation failed: %v", err)
	}
	return c, nil
}

func ValidateConfig(c *Config) error {
	if c.RequestTimeout < 0 {
		return fmt.Errorf("RequestTimeout must be positive")
	}
	if c.SleepInterval < 0 {
		return fmt.Errorf("SleepInterval must be positive")
	}
	if c.MemberFailInterval < 0 {
		return fmt.Errorf("MemberFailInterval must be positive")
	}
	if c.PGReplUser == "" {
		return fmt.Errorf("PGReplUser cannot be empty")
	}
	if c.PGReplPassword == "" {
		return fmt.Errorf("PGReplPassword cannot be empty")
	}
	if c.MaxStandbysPerSender < 1 {
		return fmt.Errorf("MaxStandbysPerSender must be at least 1")
	}
	return nil
}
