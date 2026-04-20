// Copyright 2026 PgVillage
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
	"errors"
	"fmt"
	"time"

	"github.com/pgvillage-tools/stolon/internal/util"
)

const (
	// DefaultProxyCheckInterval is the default for the interval for the proxy to check the endpoint
	DefaultProxyCheckInterval = 5 * time.Second

	// DefaultRequestTimeout is the default for a request to time out
	DefaultRequestTimeout = 10 * time.Second

	// DefaultSleepInterval is the default for sleeps during checks
	DefaultSleepInterval = 5 * time.Second

	// DefaultKeeperFailInterval sets the default for the keeper to be assumed unhealthy
	DefaultKeeperFailInterval = 20 * time.Second

	// DefaultMaxStandbysPerSender sets the default for number of standby's before cleanup
	// of old standby's is triggered.
	DefaultMaxStandbysPerSender uint = 3

	// DefaultSynchronousReplication sets the default for sync replication when not set by config
	DefaultSynchronousReplication = false

	// DefaultInitWithMultipleKeepers can be set to choose a random initial master when multiple keeper are registered
	DefaultInitWithMultipleKeepers = false

	// DefaultUsePGRewind sets the default for using PgRewind (over starting over)
	DefaultUsePGRewind = false
)

// NilConfig defines an empty config
type NilConfig struct {
	RequestTimeout          *Duration          `json:"request_timeout,omitempty"`
	SleepInterval           *Duration          `json:"sleep_interval,omitempty"`
	KeeperFailInterval      *Duration          `json:"keeper_fail_interval,omitempty"`
	MaxStandbysPerSender    *uint              `json:"max_standbys_per_sender,omitempty"`
	SynchronousReplication  *bool              `json:"synchronous_replication,omitempty"`
	InitWithMultipleKeepers *bool              `json:"init_with_multiple_keepers,omitempty"`
	UsePGRewind             *bool              `json:"use_pg_rewind,omitempty"`
	PGParameters            *map[string]string `json:"pg_parameters,omitempty"`
}

// Config defines the end result config
type Config struct {
	// Time after which any request (keepers checks from sentinel etc...) will fail.
	RequestTimeout time.Duration
	// Interval to wait before next check (for every component: keeper, sentinel, proxy).
	SleepInterval time.Duration
	// Interval after the first fail to declare a keeper as not healthy.
	KeeperFailInterval time.Duration
	// Max number of standbys for every sender. A sender can be a master or
	// another standby (with cascading replication).
	MaxStandbysPerSender uint
	// Use Synchronous replication between master and its standbys
	SynchronousReplication bool
	// Choose a random initial master when multiple keeper are registered
	InitWithMultipleKeepers bool
	// Whether to use pg_rewind
	UsePGRewind bool
	// Map of postgres parameters
	PGParameters map[string]string
}

// TODO: use json annotations instead of UnmarshalJSON

// UnmarshalJSON deserializes a NilConfig from JSON
func (c *NilConfig) UnmarshalJSON(in []byte) error {
	type Alias NilConfig
	aux := (*Alias)(c)
	if err := json.Unmarshal(in, aux); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %v", err)
	}
	return nil
}

// Copy returns a shallow copy of a NilConfig
func (c *NilConfig) Copy() *NilConfig {
	if c == nil {
		return c
	}
	var nc NilConfig
	if c.RequestTimeout != nil {
		nc.RequestTimeout = &Duration{c.RequestTimeout.Duration}
	}
	if c.SleepInterval != nil {
		nc.SleepInterval = &Duration{c.SleepInterval.Duration}
	}
	if c.KeeperFailInterval != nil {
		nc.KeeperFailInterval = &Duration{c.KeeperFailInterval.Duration}
	}
	if c.MaxStandbysPerSender != nil {
		mspr := *c.MaxStandbysPerSender
		nc.MaxStandbysPerSender = &mspr
	}
	if c.SynchronousReplication != nil {
		sr := *c.SynchronousReplication
		nc.SynchronousReplication = &sr
	}
	if c.InitWithMultipleKeepers != nil {
		iwmk := *c.InitWithMultipleKeepers
		nc.InitWithMultipleKeepers = &iwmk
	}
	if c.UsePGRewind != nil {
		upgr := *c.UsePGRewind
		nc.UsePGRewind = &upgr
	}
	if c.PGParameters != nil {
		pgp := *c.PGParameters
		nc.PGParameters = &pgp
	}
	return &nc
}

// Copy returns a shallow copy of a Config
func (c *Config) Copy() *Config {
	if c == nil {
		return c
	}
	// Just copy by dereferencing c, the PGParameters map won't be a real deep copy.
	nc := *c
	// Do a real deeep copy of the PGParameters map
	nm := map[string]string{}
	for k, v := range c.PGParameters {
		nm[k] = v
	}
	nc.PGParameters = nm
	return &nc
}

// Duration is needed to be able to marshal/unmarshal json strings with time
// unit (eg. 3s, 100ms) instead of ugly times in nanoseconds.
type Duration struct {
	time.Duration
}

// TODO convert MarshalJSON and UnmarshalJSON to using json annotations

// MarshalJSON will serialize a Duration object
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON will deserialize a Duration object
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}

// Validate can be used to validate a NilConfig
func (c *NilConfig) Validate() error {
	if c.RequestTimeout != nil && (*c.RequestTimeout).Duration < 0 {
		return errors.New("request_timeout must be positive")
	}
	if c.SleepInterval != nil && (*c.SleepInterval).Duration < 0 {
		return errors.New("sleep_interval must be positive")
	}
	if c.KeeperFailInterval != nil && (*c.KeeperFailInterval).Duration < 0 {
		return errors.New("keeper_fail_interval must be positive")
	}
	if c.MaxStandbysPerSender != nil && *c.MaxStandbysPerSender < 1 {
		return errors.New("max_standbys_per_sender must be at least 1")
	}
	return nil
}

// MergeDefaults can be used to merge in Default values
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
	if c.MaxStandbysPerSender == nil {
		c.MaxStandbysPerSender = util.ToPtr(DefaultMaxStandbysPerSender)
	}
	if c.SynchronousReplication == nil {
		c.SynchronousReplication = util.ToPtr(DefaultSynchronousReplication)
	}
	if c.InitWithMultipleKeepers == nil {
		c.InitWithMultipleKeepers = util.ToPtr(DefaultInitWithMultipleKeepers)
	}
	if c.UsePGRewind == nil {
		c.UsePGRewind = util.ToPtr(DefaultUsePGRewind)
	}
	if c.PGParameters == nil {
		c.PGParameters = &map[string]string{}
	}
}

// ToConfig converts a NilCOnfig into a Config
func (c *NilConfig) ToConfig() *Config {
	nc := c.Copy()
	nc.MergeDefaults()
	return &Config{
		RequestTimeout:          (*nc.RequestTimeout).Duration,
		SleepInterval:           (*nc.SleepInterval).Duration,
		KeeperFailInterval:      (*nc.KeeperFailInterval).Duration,
		MaxStandbysPerSender:    *nc.MaxStandbysPerSender,
		SynchronousReplication:  *nc.SynchronousReplication,
		InitWithMultipleKeepers: *nc.InitWithMultipleKeepers,
		UsePGRewind:             *nc.UsePGRewind,
		PGParameters:            *nc.PGParameters,
	}
}

// NewDefaultConfig returns a freshly initialized config
func NewDefaultConfig() *Config {
	nc := &NilConfig{}
	nc.MergeDefaults()
	return nc.ToConfig()
}
