package v0

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/stolon/internal/util"
)

var _ = Describe("Config", func() {
	var (
		tenSeconds     = Duration{10 * time.Second}
		hundredSeconds = Duration{100 * time.Second}

		mergeDefaults = func(c *NilConfig) *NilConfig {
			c.MergeDefaults()
			return c
		}
	)
	When("Parsing config", func() {
		It("should successfully parse", func() {
			const (
				defaultMaxStandbysPerSender uint = 10
			)

			for _, tt := range []struct {
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
					in: `{ "request_timeout": "3s" }`,
					cfg: mergeDefaults(&NilConfig{
						RequestTimeout: &Duration{3 * time.Second}},
					).ToConfig(),
					err: nil,
				},
				{
					in: `{ "request_timeout": "3000ms" }`,
					cfg: mergeDefaults(&NilConfig{
						RequestTimeout: &Duration{3 * time.Second}},
					).ToConfig(),
					err: nil,
				},
				{
					in:  `{ "request_timeout": "-3s" }`,
					cfg: nil,
					err: errors.New("config validation failed: request_timeout must be positive"),
				},
				{
					in:  `{ "request_timeout": "-3s" }`,
					cfg: nil,
					err: errors.New("config validation failed: request_timeout must be positive"),
				},
				{
					in:  `{ "sleep_interval": "-3s" }`,
					cfg: nil,
					err: errors.New("config validation failed: sleep_interval must be positive"),
				},
				{
					in:  `{ "keeper_fail_interval": "-3s" }`,
					cfg: nil,
					err: errors.New("config validation failed: keeper_fail_interval must be positive"),
				},
				{
					in:  `{ "max_standbys_per_sender": 0 }`,
					cfg: nil,
					err: errors.New("config validation failed: max_standbys_per_sender must be at least 1"),
				},
				// All options defined
				{
					in: strings.Join([]string{
						`{ "request_timeout": "10s", `,
						`"sleep_interval": "10s", `,
						`"keeper_fail_interval": "100s", `,
						`"max_standbys_per_sender": 5, `,
						`"synchronous_replication": true, `,
						`"init_with_multiple_keepers": true,`,
						`"pg_parameters": {`,
						`  "param01": "value01"`,
						`}}`,
					}, "\n"),
					cfg: mergeDefaults(&NilConfig{
						RequestTimeout:          &tenSeconds,
						SleepInterval:           &tenSeconds,
						KeeperFailInterval:      &hundredSeconds,
						MaxStandbysPerSender:    util.ToPtr(uint(5)),
						SynchronousReplication:  util.ToPtr(true),
						InitWithMultipleKeepers: util.ToPtr(true),
						PGParameters: &map[string]string{
							"param01": "value01",
						},
					}).ToConfig(),
					err: nil,
				},
			} {
				var nilCfg *NilConfig
				err := json.Unmarshal([]byte(tt.in), &nilCfg)
				if tt.err != nil {
					Ω(err).NotTo(BeNil())
					Ω(tt.err.Error()).To(Equal(err.Error()))
				} else {
					Ω(err).To(BeNil())
					nilCfg.MergeDefaults()
					Ω(nilCfg.ToConfig()).To(Equal(tt.cfg))
				}
			}
		})
	})
	When("Copying a nilConfig", func() {
		const (
			defaultMaxStandbysPerSender uint = 10
		)
		var (
			defaultReqTimeout          = Duration{5 * time.Second}
			defaultInterval            = Duration{6 * time.Second}
			defaultMaxStbys            = uint(5)
			defaultSyncReplicas        = true
			defaultInitMultipleKeepers = true

			newReqTimeout          = Duration{10 * time.Second}
			newInterval            = Duration{12 * time.Second}
			newMaxStbys            = uint(5)
			newSyncReplicas        = true
			newInitMultipleKeepers = true

			cfg = mergeDefaults(&NilConfig{
				RequestTimeout:          &defaultReqTimeout,
				SleepInterval:           &defaultInterval,
				KeeperFailInterval:      &defaultInterval,
				MaxStandbysPerSender:    &defaultMaxStbys,
				SynchronousReplication:  &defaultSyncReplicas,
				InitWithMultipleKeepers: &defaultInitMultipleKeepers,
				PGParameters: &map[string]string{
					"param01": "value01",
				},
			})
		)
		It("changes on copy should not impact original config", func() {
			newCfg := cfg.Copy()
			newCfg.RequestTimeout = &newReqTimeout
			newCfg.SleepInterval = &newInterval
			newCfg.KeeperFailInterval = &newInterval
			newCfg.MaxStandbysPerSender = &newMaxStbys
			newCfg.SynchronousReplication = &newSyncReplicas
			newCfg.InitWithMultipleKeepers = &newInitMultipleKeepers
			(*newCfg.PGParameters)["param01"] = "anothervalue01"

			Ω(newCfg.RequestTimeout).To(Equal(&newReqTimeout))
			Ω(newCfg.SleepInterval).To(Equal(&newInterval))
			Ω(newCfg.KeeperFailInterval).To(Equal(&newInterval))
			Ω(newCfg.MaxStandbysPerSender).To(Equal(&newMaxStbys))
			Ω(newCfg.SynchronousReplication).To(Equal(&newSyncReplicas))
			Ω(newCfg.InitWithMultipleKeepers).To(Equal(&newInitMultipleKeepers))

			Ω(cfg.RequestTimeout).To(Equal(&defaultReqTimeout))
			Ω(cfg.SleepInterval).To(Equal(&defaultInterval))
			Ω(cfg.KeeperFailInterval).To(Equal(&defaultInterval))
			Ω(cfg.MaxStandbysPerSender).To(Equal(&defaultMaxStbys))
			Ω(cfg.SynchronousReplication).To(Equal(&defaultSyncReplicas))
			Ω(cfg.InitWithMultipleKeepers).To(Equal(&defaultInitMultipleKeepers))
		})
	})
})
