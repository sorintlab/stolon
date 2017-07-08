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

package cluster

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/sorintlab/stolon/common"

	"fmt"
	"sort"

	"github.com/mitchellh/copystructure"
)

func Uint16P(u uint16) *uint16 {
	return &u
}
func Uint32P(u uint32) *uint32 {
	return &u
}

func BoolP(b bool) *bool {
	return &b
}

const (
	CurrentCDFormatVersion uint64 = 1
)

const (
	DefaultProxyCheckInterval   = 5 * time.Second
	DefaultProxyTimeoutInterval = 15 * time.Second

	DefaultDBNotIncreasingXLogPosTimes = 10

	DefaultSleepInterval                         = 5 * time.Second
	DefaultRequestTimeout                        = 10 * time.Second
	DefaultConvergenceTimeout                    = 30 * time.Second
	DefaultInitTimeout                           = 5 * time.Minute
	DefaultSyncTimeout                           = 30 * time.Minute
	DefaultFailInterval                          = 20 * time.Second
	DefaultDeadKeeperRemovalInterval             = 48 * time.Hour
	DefaultMaxStandbys               uint16      = 20
	DefaultMaxStandbysPerSender      uint16      = 3
	DefaultMaxStandbyLag                         = 1024 * 1204
	DefaultSynchronousReplication                = false
	DefaultMinSynchronousStandbys    uint16      = 1
	DefaultMaxSynchronousStandbys    uint16      = 1
	DefaultAdditionalWalSenders                  = 5
	DefaultUsePgrewind                           = false
	DefaultMergePGParameter                      = true
	DefaultRole                      ClusterRole = ClusterRoleMaster
)

const (
	NoGeneration      int64 = 0
	InitialGeneration int64 = 1
)

type PGParameters map[string]string

type FollowType string

const (
	// Follow an db managed by a keeper in our cluster
	FollowTypeInternal FollowType = "internal"
	// Follow an external db
	FollowTypeExternal FollowType = "external"
)

type FollowConfig struct {
	Type FollowType `json:"type,omitempty"`
	// Keeper ID to follow when Type is "internal"
	DBUID string `json:"dbuid,omitempty"`
	// Standby settings when Type is "external"
	StandbySettings *StandbySettings `json:"standbySettings,omitempty"`
}

type ClusterPhase string

const (
	ClusterPhaseInitializing ClusterPhase = "initializing"
	ClusterPhaseNormal       ClusterPhase = "normal"
)

type ClusterRole string

const (
	ClusterRoleMaster  ClusterRole = "master"
	ClusterRoleStandby ClusterRole = "standby"
)

type ClusterInitMode string

const (
	// Initialize a cluster starting from a freshly initialized database cluster. Valid only when cluster role is master.
	ClusterInitModeNew ClusterInitMode = "new"
	// Initialize a cluster doing a point in time recovery on a keeper.
	ClusterInitModePITR ClusterInitMode = "pitr"
	// Initialize a cluster with an user specified already populated db cluster.
	ClusterInitModeExisting ClusterInitMode = "existing"
)

func ClusterInitModeP(s ClusterInitMode) *ClusterInitMode {
	return &s
}

func ClusterRoleP(s ClusterRole) *ClusterRole {
	return &s
}

type DBInitMode string

const (
	DBInitModeNone DBInitMode = "none"
	// Use existing db cluster data
	DBInitModeExisting DBInitMode = "existing"
	// Initialize a db starting from a freshly initialized database cluster
	DBInitModeNew DBInitMode = "new"
	// Initialize a db doing a point in time recovery
	DBInitModePITR DBInitMode = "pitr"
	// Initialize a db doing a resync to a target database cluster
	DBInitModeResync DBInitMode = "resync"
)

type PITRConfig struct {
	// DataRestoreCommand defines the command to execute for restoring the db
	// cluster data). %d is replaced with the full path to the db cluster
	// datadir. Use %% to embed an actual % character.
	DataRestoreCommand      string                   `json:"dataRestoreCommand,omitempty"`
	ArchiveRecoverySettings *ArchiveRecoverySettings `json:"archiveRecoverySettings,omitempty"`
	RecoveryTargetSettings  *RecoveryTargetSettings  `json:"recoveryTargetSettings,omitempty"`
}

type ExistingConfig struct {
	KeeperUID string `json:"keeperUID,omitempty"`
}

// ArchiveRecoverySettings defines the archive recovery settings in the recovery.conf file (https://www.postgresql.org/docs/9.6/static/archive-recovery-settings.html )
type ArchiveRecoverySettings struct {
	// value for restore_command
	RestoreCommand string `json:"restoreCommand,omitempty"`
	//TODO(sgotti) add missing options
}

// RecoveryTargetSettings defines the recovery target settings in the recovery.conf file (https://www.postgresql.org/docs/9.6/static/recovery-target-settings.html )
type RecoveryTargetSettings struct {
	//TODO(sgotti) add options
}

// StandbySettings defines the standby settings in the recovery.conf file (https://www.postgresql.org/docs/9.6/static/standby-settings.html )
type StandbySettings struct {
	PrimaryConninfo       string `json:"primaryConninfo,omitempty"`
	PrimarySlotName       string `json:"primarySlotName,omitempty"`
	RecoveryMinApplyDelay string `json:"recoveryMinApplyDelay,omitempty"`
}

type ClusterSpec struct {
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
	// Interval after the first fail to declare a keeper or a db as not healthy.
	FailInterval *Duration `json:"failInterval,omitempty"`
	// Interval after which a dead keeper will be removed from the cluster data
	DeadKeeperRemovalInterval *Duration `json:"deadKeeperRemovalInterval,omitempty"`
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
	MaxStandbyLag *uint32 `json:"maxStandbyLage,omitempty"`
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
	// Whether to use pg_rewind
	UsePgrewind *bool `json:"usePgrewind,omitempty"`
	// InitMode defines the cluster initialization mode. Current modes are: new, existing, pitr
	InitMode *ClusterInitMode `json:"initMode,omitempty"`
	// Whether to merge pgParameters of the initialized db cluster, useful
	// the retain initdb generated parameters when InitMode is new, retain
	// current parameters when initMode is existing or pitr.
	MergePgParameters *bool `json:"mergePgParameters,omitempty"`
	// Role defines the cluster operating role (master or standby of an external database)
	Role *ClusterRole `json:"role,omitempty"`
	// Point in time recovery init configuration used when InitMode is "pitr"
	PITRConfig *PITRConfig `json:"pitrConfig,omitempty"`
	// Existing init configuration used when InitMode is "existing"
	ExistingConfig *ExistingConfig `json:"existingConfig,omitempty"`
	// Standby setting when role is standby
	StandbySettings *StandbySettings `json:"standbySettings,omitempty"`
	// Map of postgres parameters
	PGParameters PGParameters `json:"pgParameters,omitempty"`
}

type ClusterStatus struct {
	CurrentGeneration int64        `json:"currentGeneration,omitempty"`
	Phase             ClusterPhase `json:"phase,omitempty"`
	// Master DB UID
	Master string `json:"master,omitempty"`
}

type Cluster struct {
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *ClusterSpec `json:"spec,omitempty"`

	Status ClusterStatus `json:"status,omitempty"`
}

func (c *Cluster) DeepCopy() *Cluster {
	nc, err := copystructure.Copy(c)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(c, nc) {
		panic("not equal")
	}
	return nc.(*Cluster)
}

func (c *ClusterSpec) DeepCopy() *ClusterSpec {
	nc, err := copystructure.Copy(c)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(c, nc) {
		panic("not equal")
	}
	return nc.(*ClusterSpec)
}

// DefSpec returns a new ClusterSpec with unspecified values populated with
// their defaults
func (c *Cluster) DefSpec() *ClusterSpec {
	return c.Spec.WithDefaults()
}

// WithDefaults returns a new ClusterSpec with unspecified values populated with
// their defaults
func (os *ClusterSpec) WithDefaults() *ClusterSpec {
	// Take a copy of the input ClusterSpec since we don't want to change the original
	s := os.DeepCopy()
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
	if s.FailInterval == nil {
		s.FailInterval = &Duration{Duration: DefaultFailInterval}
	}
	if s.DeadKeeperRemovalInterval == nil {
		s.DeadKeeperRemovalInterval = &Duration{Duration: DefaultDeadKeeperRemovalInterval}
	}
	if s.MaxStandbys == nil {
		s.MaxStandbys = Uint16P(DefaultMaxStandbys)
	}
	if s.MaxStandbysPerSender == nil {
		s.MaxStandbysPerSender = Uint16P(DefaultMaxStandbysPerSender)
	}
	if s.MaxStandbyLag == nil {
		s.MaxStandbyLag = Uint32P(DefaultMaxStandbyLag)
	}
	if s.SynchronousReplication == nil {
		s.SynchronousReplication = BoolP(DefaultSynchronousReplication)
	}
	if s.UsePgrewind == nil {
		s.UsePgrewind = BoolP(DefaultUsePgrewind)
	}
	if s.MinSynchronousStandbys == nil {
		s.MinSynchronousStandbys = Uint16P(DefaultMinSynchronousStandbys)
	}
	if s.MaxSynchronousStandbys == nil {
		s.MaxSynchronousStandbys = Uint16P(DefaultMaxSynchronousStandbys)
	}
	if s.AdditionalWalSenders == nil {
		s.AdditionalWalSenders = Uint16P(DefaultAdditionalWalSenders)
	}
	if s.MergePgParameters == nil {
		s.MergePgParameters = BoolP(DefaultMergePGParameter)
	}
	if s.Role == nil {
		v := DefaultRole
		s.Role = &v
	}
	return s
}

// Validate validates a cluster spec.
func (os *ClusterSpec) Validate() error {
	s := os.WithDefaults()
	if s.SleepInterval.Duration < 0 {
		return fmt.Errorf("sleepInterval must be positive")
	}
	if s.RequestTimeout.Duration < 0 {
		return fmt.Errorf("requestTimeout must be positive")
	}
	if s.ConvergenceTimeout.Duration < 0 {
		return fmt.Errorf("convergenceTimeout must be positive")
	}
	if s.InitTimeout.Duration < 0 {
		return fmt.Errorf("initTimeout must be positive")
	}
	if s.SyncTimeout.Duration < 0 {
		return fmt.Errorf("syncTimeout must be positive")
	}
	if s.DeadKeeperRemovalInterval.Duration < 0 {
		return fmt.Errorf("deadKeeperRemovalInterval must be positive")
	}
	if s.FailInterval.Duration < 0 {
		return fmt.Errorf("failInterval must be positive")
	}
	if *s.MaxStandbys < 1 {
		return fmt.Errorf("maxStandbys must be at least 1")
	}
	if *s.MaxStandbysPerSender < 1 {
		return fmt.Errorf("maxStandbysPerSender must be at least 1")
	}
	if *s.MinSynchronousStandbys < 1 {
		return fmt.Errorf("minSynchronousStandbys must be at least 1")
	}
	if *s.MaxSynchronousStandbys < 1 {
		return fmt.Errorf("maxSynchronousStandbys must be at least 1")
	}
	if *s.MaxSynchronousStandbys < *s.MinSynchronousStandbys {
		return fmt.Errorf("maxSynchronousStandbys must be greater or equal to minSynchronousStandbys")
	}
	if s.InitMode == nil {
		return fmt.Errorf("initMode undefined")
	}
	switch *s.InitMode {
	case ClusterInitModeNew:
		if *s.Role == ClusterRoleStandby {
			return fmt.Errorf("invalid cluster role standby when initMode is \"new\"")
		}
	case ClusterInitModeExisting:
		if s.ExistingConfig == nil {
			return fmt.Errorf("existingConfig undefined. Required when initMode is \"existing\"")
		}
		if s.ExistingConfig.KeeperUID == "" {
			return fmt.Errorf("existingConfig.keeperUID undefined")
		}
	case ClusterInitModePITR:
		if s.PITRConfig == nil {
			return fmt.Errorf("pitrConfig undefined. Required when initMode is \"pitr\"")
		}
		if s.PITRConfig.DataRestoreCommand == "" {
			return fmt.Errorf("pitrConfig.DataRestoreCommand undefined")
		}
	default:
		return fmt.Errorf("unknown initMode: %q", *s.InitMode)

	}

	switch *s.Role {
	case ClusterRoleMaster:
	case ClusterRoleStandby:
		if s.StandbySettings == nil {
			return fmt.Errorf("standbySettings undefined. Required when cluster role is \"standby\"")
		}
		if s.StandbySettings.PrimaryConninfo == "" {
			return fmt.Errorf("standbySettings primaryConnInfo undefined. Required when cluster role is \"standby\"")
		}
	default:
		return fmt.Errorf("unknown role: %q", *s.InitMode)
	}
	return nil
}

func (c *Cluster) UpdateSpec(ns *ClusterSpec) error {
	s := c.Spec
	if err := ns.Validate(); err != nil {
		return fmt.Errorf("invalid cluster spec: %v", err)
	}
	ds := s.WithDefaults()
	dns := ns.WithDefaults()
	if *ds.InitMode != *dns.InitMode {
		return fmt.Errorf("cannot change cluster init mode")
	}
	if *ds.Role == ClusterRoleMaster && *dns.Role == ClusterRoleStandby {
		return fmt.Errorf("cannot update a cluster from master role to standby role")
	}
	c.Spec = ns
	return nil
}

func NewCluster(uid string, cs *ClusterSpec) *Cluster {
	c := &Cluster{
		UID:        uid,
		Generation: InitialGeneration,
		ChangeTime: time.Now(),
		Spec:       cs,
		Status: ClusterStatus{
			Phase: ClusterPhaseInitializing,
		},
	}
	return c
}

type KeeperSpec struct{}

type KeeperStatus struct {
	Healthy         bool      `json:"healthy,omitempty"`
	LastHealthyTime time.Time `json:"lastHealthyTime,omitempty"`

	BootUUID string `json:"bootUUID,omitempty"`
}

type Keeper struct {
	// Keeper ID
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *KeeperSpec `json:"spec,omitempty"`

	Status KeeperStatus `json:"status,omitempty"`
}

func NewKeeperFromKeeperInfo(ki *KeeperInfo) *Keeper {
	return &Keeper{
		UID:        ki.UID,
		Generation: InitialGeneration,
		ChangeTime: time.Time{},
		Spec:       &KeeperSpec{},
		Status: KeeperStatus{
			Healthy:         true,
			LastHealthyTime: time.Now(),
			BootUUID:        ki.BootUUID,
		},
	}
}

func (kss Keepers) SortedKeys() []string {
	keys := []string{}
	for k, _ := range kss {
		keys = append(keys, k)
	}
	sort.Sort(sort.StringSlice(keys))
	return keys
}

type DBSpec struct {
	// The KeeperUID this db is assigned to
	KeeperUID string `json:"keeperUID,omitempty"`
	// Time after which any request (keepers checks from sentinel etc...) will fail.
	RequestTimeout Duration `json:"requestTimeout,omitempty"`
	// See ClusterSpec MaxStandbys description
	MaxStandbys uint16 `json:"maxStandbys,omitempty"`
	// Use Synchronous replication between master and its standbys
	SynchronousReplication bool `json:"synchronousReplication,omitempty"`
	// Whether to use pg_rewind
	UsePgrewind bool `json:"usePgrewind,omitempty"`
	// AdditionalWalSenders defines the number of additional wal_senders in
	// addition to the ones internally defined by stolon
	AdditionalWalSenders uint16 `json:"additionalWalSenders"`
	// InitMode defines the db initialization mode. Current modes are: none, new
	InitMode DBInitMode `json:"initMode,omitempty"`
	// Point in time recovery init configuration used when InitMode is "pitr"
	PITRConfig *PITRConfig `json:"pitrConfig,omitempty"`
	// Map of postgres parameters
	PGParameters PGParameters `json:"pgParameters,omitempty"`
	// DB Role (master or standby)
	Role common.Role `json:"role,omitempty"`
	// FollowConfig when Role is "standby"
	FollowConfig *FollowConfig `json:"followConfig,omitempty"`
	// Followers DB UIDs
	Followers []string `json:"followers"`
	// Whether to include previous postgresql.conf
	IncludeConfig bool `json:"includePreviousConfig,omitempty"`
	// SynchronousStandbys are the standbys to be configured as synchronous
	SynchronousStandbys []string `json:"synchronousStandbys"`
}

type DBStatus struct {
	Healthy bool `json:"healthy,omitempty"`

	CurrentGeneration int64 `json:"currentGeneration,omitempty"`

	ListenAddress string `json:"listenAddress,omitempty"`
	Port          string `json:"port,omitempty"`

	SystemID         string                   `json:"systemdID,omitempty"`
	TimelineID       uint64                   `json:"timelineID,omitempty"`
	XLogPos          uint64                   `json:"xLogPos,omitempty"`
	TimelinesHistory PostgresTimelinesHistory `json:"timelinesHistory,omitempty"`

	PGParameters        PGParameters `json:"pgParameters,omitempty"`
	SynchronousStandbys []string     `json:"synchronousStandbys"`
	OlderWalFile        string       `json:"olderWalFile,omitempty"`
}

type DB struct {
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *DBSpec `json:"spec,omitempty"`

	Status DBStatus `json:"status,omitempty"`
}

type ProxySpec struct {
	MasterDBUID    string   `json:"masterDbUid,omitempty"`
	EnabledProxies []string `json:"enabledProxies,omitempty"`
}

type ProxyStatus struct {
}

type Proxy struct {
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec ProxySpec `json:"spec,omitempty"`

	Status ProxyStatus `json:"status,omitempty"`
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

type Keepers map[string]*Keeper
type DBs map[string]*DB

// for simplicity keep all the changes to the various components atomic (using
// an unique key)
type ClusterData struct {
	// ClusterData format version. Used to detect incompatible
	// version and do upgrade. Needs to be bumped when a non
	// backward compatible change is done to the other struct
	// members.
	FormatVersion uint64
	Cluster       *Cluster
	Keepers       Keepers
	DBs           DBs
	Proxy         *Proxy
}

func NewClusterData(c *Cluster) *ClusterData {
	return &ClusterData{
		FormatVersion: CurrentCDFormatVersion,
		Cluster:       c,
		Keepers:       make(Keepers),
		DBs:           make(DBs),
		Proxy:         &Proxy{},
	}
}

func (c *ClusterData) DeepCopy() *ClusterData {
	nc, err := copystructure.Copy(c)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(c, nc) {
		panic("not equal")
	}
	return nc.(*ClusterData)
}

func (cd *ClusterData) FindDB(keeper *Keeper) *DB {
	for _, db := range cd.DBs {
		if db.Spec.KeeperUID == keeper.UID {
			return db
		}
	}
	return nil
}
