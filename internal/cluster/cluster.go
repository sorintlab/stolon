// Copyright 2016 Sorint.lab
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

// Package cluster defines all resources used at the cluster level
package cluster

// TODO: Split into multiple modules
import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mitchellh/copystructure"
	"github.com/sorintlab/stolon/internal/common"
	"github.com/sorintlab/stolon/internal/postgresql"
	"github.com/sorintlab/stolon/internal/util"
)

// TODO: Rename XLOG to WAL everywhere

const (
	// CurrentCDFormatVersion represents the current version of the Cluster Data Format.
	// It will be raised whenever we change the API
	CurrentCDFormatVersion uint64 = 1

	// DefaultStoreTimeout sets the default for a store timeout
	DefaultStoreTimeout = 5 * time.Second

	// DefaultDBNotIncreasingXLogPosTimes sets a default for the number of checks that it is ok for stolon
	// not to detect XLog position increases on a standby. If WAL position is not increased more then this value,
	// the standby is assumed not to be syncing properly.
	DefaultDBNotIncreasingXLogPosTimes = 10

	// DefaultSleepInterval is the default for sleeps during checks
	DefaultSleepInterval = 5 * time.Second

	// DefaultRequestTimeout is the default for a request to time out
	DefaultRequestTimeout = 10 * time.Second

	// DefaultConvergenceTimeout sets the timeout for convergence (of primaries and replica's) to be successful.
	DefaultConvergenceTimeout = 30 * time.Second

	// DefaultInitTimeout sets the default timeout fo initializing a new cluster
	DefaultInitTimeout = 5 * time.Minute

	// DefaultSyncTimeout sets the default timeout for waiting for a database recovery
	// (including the replay of WAL files in case of Point-In-Time-Recovery)
	DefaultSyncTimeout = 0

	// DefaultDBWaitReadyTimeout sets the default for Ready status (being able to connect to PostgreSQL)
	DefaultDBWaitReadyTimeout = 60 * time.Second

	// DefaultFailInterval sets the default for the cluster to be assumed unhealthy
	DefaultFailInterval = 20 * time.Second

	// DefaultDeadKeeperRemovalInterval is the interval after which a keeper is assumed dead and is removed from the
	// config.
	DefaultDeadKeeperRemovalInterval = 48 * time.Hour

	// DefaultProxyCheckInterval is the default for the interval for the proxy to check the endpoint
	DefaultProxyCheckInterval = 5 * time.Second

	// DefaultProxyTimeout is the default for the proxy check timeout. Once expired, all connections will be closed.
	DefaultProxyTimeout = 15 * time.Second

	// DefaultMaxStandbys sets the default for teh number of standby's per keeper (input for max_replication_slots and
	// max_wal_senders)
	DefaultMaxStandbys uint16 = 20

	// DefaultMaxStandbysPerSender sets the default for number of standby's before cleanup of old standby's is triggered
	DefaultMaxStandbysPerSender uint16 = 3

	// DefaultMaxStandbyLag sets the default for maximum standby lag for sync replica's (1MiB).
	// Replicas with more lag are assumed not to be valid sync replica's (yet)
	DefaultMaxStandbyLag = 1024 * 1204

	// DefaultSynchronousReplication sets the default for sync replication when not set by config
	DefaultSynchronousReplication = false

	// DefaultMinSynchronousStandbys sets the default for minimum number of sync standby's
	DefaultMinSynchronousStandbys uint16 = 1

	// DefaultMaxSynchronousStandbys sets the default for maximum number of sync standby's
	DefaultMaxSynchronousStandbys uint16 = 1

	// DefaultAdditionalWalSenders sets the default for additional wal-senders on top of as required for stolon managed
	DefaultAdditionalWalSenders = 5

	// DefaultUsePgrewind sets the default for using PgRewind (over starting over)
	// TODO: enable pgrewind by default
	DefaultUsePgrewind = false

	// DefaultMergePGParameter sets the default for merging parameters from local cluster to config after initialization
	DefaultMergePGParameter = true

	// DefaultRole sets the default role for this cluster. A primary cluster has a primary, a Standby has only replica's
	// and is replicating from another cluster
	DefaultRole Role = Primary

	// DefaultSUReplAccess sets the default for replication access (strict: only from other standby's in this cluster to
	// primary; all: any replication conenction is accepted when properly authenticated)
	DefaultSUReplAccess SUReplAccessMode = SUReplAccessAll

	// DefaultAutomaticPgRestart sets the default for restarting postgres when required to apply config changes
	DefaultAutomaticPgRestart = false
)

const (
	// NoGeneration is the default Generation when not converging
	NoGeneration int64 = 0
	// InitialGeneration is the default generation when starting convergence
	InitialGeneration int64 = 1
)

// PGParameters is a map of all PostgreSQL config as configured for this cluster
type PGParameters map[string]string

// PostgresBinaryVersion specifies the PostgreSQL binary version (major / minor)
type PostgresBinaryVersion struct {
	Maj int
	Min int
}

// NewConfig  defines the config for a new Cluster
type NewConfig struct {
	Locale        string `json:"locale,omitempty"`
	Encoding      string `json:"encoding,omitempty"`
	DataChecksums bool   `json:"dataChecksums,omitempty"`
}

// PITRConfig defines the config for restoring using Point in time Recovery
type PITRConfig struct {
	// DataRestoreCommand defines the command to execute for restoring the db
	// cluster data). %d is replaced with the full path to the db cluster
	// datadir. Use %% to embed an actual % character.
	DataRestoreCommand      string                   `json:"dataRestoreCommand,omitempty"`
	ArchiveRecoverySettings *ArchiveRecoverySettings `json:"archiveRecoverySettings,omitempty"`
	RecoveryTargetSettings  *RecoveryTargetSettings  `json:"recoveryTargetSettings,omitempty"`
}

// ExistingConfig defines the config for resuing an existing DB
type ExistingConfig struct {
	KeeperUID string `json:"keeperUID,omitempty"`
}

// StandbyConfig defines the config to be used for Replicas
type StandbyConfig struct {
	StandbySettings         *StandbySettings         `json:"standbySettings,omitempty"`
	ArchiveRecoverySettings *ArchiveRecoverySettings `json:"archiveRecoverySettings,omitempty"`
}

// ArchiveRecoverySettings defines the archive recovery settings in the recovery.conf file
// (https://www.postgresql.org/docs/9.6/static/archive-recovery-settings.html )
type ArchiveRecoverySettings struct {
	// value for restore_command
	RestoreCommand string `json:"restoreCommand,omitempty"`
}

// RecoveryTargetSettings defines the recovery target settings in the recovery.conf file
// (https://www.postgresql.org/docs/9.6/static/recovery-target-settings.html )
type RecoveryTargetSettings struct {
	RecoveryTarget         string `json:"recoveryTarget,omitempty"`
	RecoveryTargetLsn      string `json:"recoveryTargetLsn,omitempty"`
	RecoveryTargetName     string `json:"recoveryTargetName,omitempty"`
	RecoveryTargetTime     string `json:"recoveryTargetTime,omitempty"`
	RecoveryTargetXid      string `json:"recoveryTargetXid,omitempty"`
	RecoveryTargetTimeline string `json:"recoveryTargetTimeline,omitempty"`
}

// StandbySettings defines the standby settings in the recovery.conf file
// (https://www.postgresql.org/docs/9.6/static/standby-settings.html )
type StandbySettings struct {
	PrimaryConninfo       string `json:"primaryConninfo,omitempty"`
	PrimarySlotName       string `json:"primarySlotName,omitempty"`
	RecoveryMinApplyDelay string `json:"recoveryMinApplyDelay,omitempty"`
}

// SUReplAccessMode is an ENUM for Access Mode for Superusers (in HBA)
type SUReplAccessMode string

const (
	// SUReplAccessAll allows access from every host
	SUReplAccessAll SUReplAccessMode = "all"
	// SUReplAccessStrict allows access from standby server IPs only
	SUReplAccessStrict SUReplAccessMode = "strict"
)

// Spec holds the cluster wide configuration
type Spec struct {
	// Interval to wait before next check
	SleepInterval *Duration `json:"sleepInterval,omitempty"`
	// Time after which any request (keepers checks from sentinel etc...) will fail.
	RequestTimeout *Duration `json:"requestTimeout,omitempty"`
	// Interval to wait for a db to be converged to the required state when
	// no long operation are expected.
	ConvergenceTimeout *Duration `json:"convergenceTimeout,omitempty"`
	// Interval to wait for a db to be initialized (doing a initdb)
	InitTimeout *Duration `json:"initTimeout,omitempty"`
	// Interval to wait for a db to be synced with a master
	SyncTimeout *Duration `json:"syncTimeout,omitempty"`
	// Interval to wait for a db to boot and become ready
	DBWaitReadyTimeout *Duration `json:"dbWaitReadyTimeout,omitempty"`
	// Interval after the first fail to declare a keeper or a db as not healthy.
	FailInterval *Duration `json:"failInterval,omitempty"`
	// Interval after which a dead keeper will be removed from the cluster data
	DeadKeeperRemovalInterval *Duration `json:"deadKeeperRemovalInterval,omitempty"`
	// Interval to wait before next proxy check
	ProxyCheckInterval *Duration `json:"proxyCheckInterval,omitempty"`
	// Interval where the proxy must successfully complete a check
	ProxyTimeout *Duration `json:"proxyTimeout,omitempty"`
	// Max number of standbys. This needs to be greater enough to cover both
	// standby managed by stolon and additional standbys configured by the
	// user. Its value affect different postgres parameters like
	// max_replication_slots and max_wal_senders. Setting this to a number
	// lower than the sum of stolon managed standbys and user managed
	// standbys will have unpredicatable effects due to problems creating
	// replication slots or replication problems due to exhausted wal
	// senders.
	MaxStandbys *uint16 `json:"maxStandbys,omitempty"`
	// Max number of standbys for every sender. A sender can be a master or
	// another standby (if/when implementing cascading replication).
	MaxStandbysPerSender *uint16 `json:"maxStandbysPerSender,omitempty"`
	// Max lag in bytes that an asynchronous standy can have to be elected in
	// place of a failed master
	MaxStandbyLag *uint32 `json:"maxStandbyLag,omitempty"`
	// Use Synchronous replication between master and its standbys
	SynchronousReplication *bool `json:"synchronousReplication,omitempty"`
	// MinSynchronousStandbys is the mininum number if synchronous standbys
	// to be configured when SynchronousReplication is true
	MinSynchronousStandbys *uint16 `json:"minSynchronousStandbys,omitempty"`
	// MaxSynchronousStandbys is the maximum number if synchronous standbys
	// to be configured when SynchronousReplication is true
	MaxSynchronousStandbys *uint16 `json:"maxSynchronousStandbys,omitempty"`
	// AdditionalWalSenders defines the number of additional wal_senders in
	// addition to the ones internally defined by stolon
	AdditionalWalSenders *uint16 `json:"additionalWalSenders"`
	// AdditionalMasterReplicationSlots defines additional replication slots to
	// be created on the master postgres instance. Replication slots not defined
	// here will be dropped from the master instance (i.e. manually created
	// replication slots will be removed).
	AdditionalMasterReplicationSlots []string `json:"additionalMasterReplicationSlots"`
	// Whether to use pg_rewind
	UsePgrewind *bool `json:"usePgrewind,omitempty"`
	// InitMode defines the cluster initialization mode. Current modes are: new, existing, pitr
	InitMode *InitMode `json:"initMode,omitempty"`
	// Whether to merge pgParameters of the initialized db cluster, useful
	// the retain initdb generated parameters when InitMode is new, retain
	// current parameters when initMode is existing or pitr.
	MergePgParameters *bool `json:"mergePgParameters,omitempty"`
	// Role defines the cluster operating role (master or standby of an external database)
	Role *Role `json:"role,omitempty"`
	// Init configuration used when InitMode is "new"
	NewConfig *NewConfig `json:"newConfig,omitempty"`
	// Point in time recovery init configuration used when InitMode is "pitr"
	PITRConfig *PITRConfig `json:"pitrConfig,omitempty"`
	// Existing init configuration used when InitMode is "existing"
	ExistingConfig *ExistingConfig `json:"existingConfig,omitempty"`
	// Standby config when role is standby
	StandbyConfig *StandbyConfig `json:"standbyConfig,omitempty"`
	// Define the mode of the default hba rules needed for replication by standby keepers
	// (the su and repl auth methods will be the one provided in the keeper command line options)
	// Values can be "all" or "strict", "all" allow access from all ips, "strict" restrict
	// master access to standby servers ips.
	// Default is "all"
	DefaultSUReplAccessMode *SUReplAccessMode `json:"defaultSUReplAccessMode,omitempty"`
	// Map of postgres parameters
	PGParameters PGParameters `json:"pgParameters,omitempty"`
	// Additional pg_hba.conf entries
	// we don't set omitempty since we want to distinguish between null or empty slice
	PGHBA []string `json:"pgHBA"`
	// Enable automatic pg restart when pg parameters that requires restart changes
	AutomaticPgRestart *bool `json:"automaticPgRestart"`
}

// Status stores the status info for this cluster
type Status struct {
	CurrentGeneration int64 `json:"currentGeneration,omitempty"`
	Phase             Phase `json:"phase,omitempty"`
	// Master DB UID
	Master string `json:"master,omitempty"`
}

// Cluster wraps cluster config, cluster status, UUID, etc together
type Cluster struct {
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *Spec `json:"spec,omitempty"`

	Status Status `json:"status,omitempty"`
}

// DeepCopy copies the entire structure and returns a complete clone
func (c *Cluster) DeepCopy() (dc *Cluster) {
	var ok bool
	if nc, err := copystructure.Copy(c); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(c, nc) {
		panic("not equal")
	} else if dc, ok = nc.(*Cluster); !ok {
		panic("different type after copy")
	}
	return dc
}

// DeepCopy copies the entire structure and returns a complete clone
func (s *Spec) DeepCopy() (dc *Spec) {
	var ok bool
	if nc, err := copystructure.Copy(s); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(s, nc) {
		panic("not equal")
	} else if dc, ok = nc.(*Spec); !ok {
		panic("different type after copy")
	}
	return dc
}

// DefSpec returns a new Spec with unspecified values populated with
// their defaults
func (c *Cluster) DefSpec() *Spec {
	s := c.Spec
	if s == nil {
		s = &Spec{}
	}
	return s.WithDefaults()
}

// WithDefaults returns a new Spec with unspecified values populated with
// their defaults
func (s *Spec) WithDefaults() *Spec {
	// Take a copy of the input Spec since we don't want to change the original
	s = s.DeepCopy()
	if s.SleepInterval == nil {
		s.SleepInterval = &Duration{Duration: DefaultSleepInterval}
	}
	if s.RequestTimeout == nil {
		s.RequestTimeout = &Duration{Duration: DefaultRequestTimeout}
	}
	if s.ConvergenceTimeout == nil {
		s.ConvergenceTimeout = &Duration{Duration: DefaultConvergenceTimeout}
	}
	if s.InitTimeout == nil {
		s.InitTimeout = &Duration{Duration: DefaultInitTimeout}
	}
	if s.SyncTimeout == nil {
		s.SyncTimeout = &Duration{Duration: DefaultSyncTimeout}
	}
	if s.DBWaitReadyTimeout == nil {
		s.DBWaitReadyTimeout = &Duration{Duration: DefaultDBWaitReadyTimeout}
	}
	if s.FailInterval == nil {
		s.FailInterval = &Duration{Duration: DefaultFailInterval}
	}
	if s.DeadKeeperRemovalInterval == nil {
		s.DeadKeeperRemovalInterval = &Duration{Duration: DefaultDeadKeeperRemovalInterval}
	}
	if s.ProxyCheckInterval == nil {
		s.ProxyCheckInterval = &Duration{Duration: DefaultProxyCheckInterval}
	}
	if s.ProxyTimeout == nil {
		s.ProxyTimeout = &Duration{Duration: DefaultProxyTimeout}
	}
	if s.MaxStandbys == nil {
		s.MaxStandbys = util.ToPtr(DefaultMaxStandbys)
	}
	if s.MaxStandbysPerSender == nil {
		s.MaxStandbysPerSender = util.ToPtr(DefaultMaxStandbysPerSender)
	}
	if s.MaxStandbyLag == nil {
		s.MaxStandbyLag = util.ToPtr(uint32(DefaultMaxStandbyLag))
	}
	if s.SynchronousReplication == nil {
		s.SynchronousReplication = util.ToPtr(DefaultSynchronousReplication)
	}
	if s.UsePgrewind == nil {
		s.UsePgrewind = util.ToPtr(DefaultUsePgrewind)
	}
	if s.MinSynchronousStandbys == nil {
		s.MinSynchronousStandbys = util.ToPtr(DefaultMinSynchronousStandbys)
	}
	if s.MaxSynchronousStandbys == nil {
		s.MaxSynchronousStandbys = util.ToPtr(DefaultMaxSynchronousStandbys)
	}
	if s.AdditionalWalSenders == nil {
		s.AdditionalWalSenders = util.ToPtr(uint16(DefaultAdditionalWalSenders))
	}
	if s.MergePgParameters == nil {
		s.MergePgParameters = util.ToPtr(DefaultMergePGParameter)
	}
	if s.DefaultSUReplAccessMode == nil {
		v := DefaultSUReplAccess
		s.DefaultSUReplAccessMode = &v
	}
	if s.Role == nil {
		v := DefaultRole
		s.Role = &v
	}
	if s.AutomaticPgRestart == nil {
		s.AutomaticPgRestart = util.ToPtr(DefaultAutomaticPgRestart)
	}
	return s
}

// Validate validates a cluster spec.
func (s *Spec) Validate() error {
	s = s.WithDefaults()
	if s.SleepInterval.Duration < 0 {
		return errors.New("sleepInterval must be positive")
	}
	if s.RequestTimeout.Duration < 0 {
		return errors.New("requestTimeout must be positive")
	}
	if s.ConvergenceTimeout.Duration < 0 {
		return errors.New("convergenceTimeout must be positive")
	}
	if s.InitTimeout.Duration < 0 {
		return errors.New("initTimeout must be positive")
	}
	if s.SyncTimeout.Duration < 0 {
		return errors.New("syncTimeout must be positive")
	}
	if s.DBWaitReadyTimeout.Duration < 0 {
		return errors.New("dbWaitReadyTimeout must be positive")
	}
	if s.FailInterval.Duration < 0 {
		return errors.New("failInterval must be positive")
	}
	if s.DeadKeeperRemovalInterval.Duration < 0 {
		return errors.New("deadKeeperRemovalInterval must be positive")
	}
	if s.ProxyCheckInterval.Duration < 0 {
		return errors.New("proxyCheckInterval must be positive")
	}
	if s.ProxyTimeout.Duration < 0 {
		return errors.New("proxyTimeout must be positive")
	}
	if s.ProxyCheckInterval.Duration >= s.ProxyTimeout.Duration {
		return errors.New("proxyCheckInterval should be less than proxyTimeout")
	}
	if *s.MaxStandbys < 1 {
		return errors.New("maxStandbys must be at least 1")
	}
	if *s.MaxStandbysPerSender < 1 {
		return errors.New("maxStandbysPerSender must be at least 1")
	}
	if *s.MaxSynchronousStandbys < 1 {
		return errors.New("maxSynchronousStandbys must be at least 1")
	}
	if *s.MaxSynchronousStandbys < *s.MinSynchronousStandbys {
		return errors.New("maxSynchronousStandbys must be greater or equal to minSynchronousStandbys")
	}
	if s.InitMode == nil {
		return errors.New("initMode undefined")
	}
	for _, replicationSlot := range s.AdditionalMasterReplicationSlots {
		if err := validateReplicationSlot(replicationSlot); err != nil {
			return err
		}
	}

	// The unique validation we're doing on pgHBA entries is that they don't contain a newline character
	for _, e := range s.PGHBA {
		if strings.Contains(e, "\n") {
			return errors.New("pgHBA entries cannot contain newline characters")
		}
	}

	switch *s.InitMode {
	case New:
		if *s.Role == Replica {
			return errors.New("invalid cluster role standby when initMode is \"new\"")
		}
	case ExistingCluster:
		if s.ExistingConfig == nil {
			return errors.New("existingConfig undefined. Required when initMode is \"existing\"")
		}
		if s.ExistingConfig.KeeperUID == "" {
			return errors.New("existingConfig.keeperUID undefined")
		}
	case PITR:
		if s.PITRConfig == nil {
			return errors.New("pitrConfig undefined. Required when initMode is \"pitr\"")
		}
		if s.PITRConfig.DataRestoreCommand == "" {
			return errors.New("pitrConfig.DataRestoreCommand undefined")
		}
		if s.PITRConfig.RecoveryTargetSettings != nil && *s.Role == Replica {
			return errors.New("cannot define pitrConfig.RecoveryTargetSettings when required cluster role is standby")
		}
	default:
		return fmt.Errorf("unknown initMode: %q", *s.InitMode)
	}

	switch *s.DefaultSUReplAccessMode {
	case SUReplAccessAll:
	case SUReplAccessStrict:
	default:
		return fmt.Errorf("unknown defaultSUReplAccessMode: %q", *s.DefaultSUReplAccessMode)
	}

	switch *s.Role {
	case Primary:
	case Replica:
		if s.StandbyConfig == nil {
			return errors.New("standbyConfig undefined. Required when cluster role is \"standby\"")
		}
	default:
		return fmt.Errorf("unknown role: %q", *s.InitMode)
	}
	return nil
}

func validateReplicationSlot(replicationSlot string) error {
	if !postgresql.IsValidReplSlotName(replicationSlot) {
		return fmt.Errorf("wrong replication slot name: %q", replicationSlot)
	}
	if common.IsStolonName(replicationSlot) {
		return fmt.Errorf("replication slot name is reserved: %q", replicationSlot)
	}
	return nil
}

// UpdateSpec will update the spec of a cluster
func (c *Cluster) UpdateSpec(ns *Spec) error {
	s := c.Spec
	if err := ns.Validate(); err != nil {
		return fmt.Errorf("invalid cluster spec: %v", err)
	}
	ds := s.WithDefaults()
	dns := ns.WithDefaults()
	if *ds.InitMode != *dns.InitMode {
		return errors.New("cannot change cluster init mode")
	}
	if *ds.Role == Primary && *dns.Role == Replica {
		return errors.New("cannot update a cluster from master role to standby role")
	}
	c.Spec = ns
	return nil
}

// NewCluster returns a freshly initialized cluster object
func NewCluster(uid string, cs *Spec) *Cluster {
	c := &Cluster{
		UID:        uid,
		Generation: InitialGeneration,
		ChangeTime: time.Now(),
		Spec:       cs,
		Status: Status{
			Phase: Initializing,
		},
	}
	return c
}
