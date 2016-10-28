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
	"strings"
	"time"

	"github.com/sorintlab/stolon/common"

	"fmt"
	"sort"

	"github.com/mitchellh/copystructure"
)

const (
	CurrentCDFormatVersion uint64 = 1
)

const (
	DefaultProxyCheckInterval = 5 * time.Second

	DefaultSleepInterval               = 5 * time.Second
	DefaultRequestTimeout              = 10 * time.Second
	DefaultConvergenceTimeout          = 30 * time.Second
	DefaultInitTimeout                 = 5 * time.Minute
	DefaultFailInterval                = 20 * time.Second
	DefaultMaxStandbys          uint16 = 20
	DefaultMaxStandbysPerSender uint16 = 3
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
	// Initialize a cluster starting from a freshly initialized database cluster
	ClusterInitModeNew ClusterInitMode = "new"
	// Initialize a cluster doing a point in time recovery on a keeper. The other keepers will sync with it.
	ClusterInitModePITR ClusterInitMode = "pitr"
	// Initialize a cluster with an user specified already populated db cluster. The other keepers will sync with it.
	ClusterInitModeExisting ClusterInitMode = "existing"
)

type DBInitMode string

const (
	// No db initialization
	DBInitModeNone DBInitMode = "none"
	// Initialize a db starting from a freshly initialized database cluster
	DBInitModeNew DBInitMode = "new"
	// Initialize a db doing a point in time recovery.
	DBInitModePITR DBInitMode = "pitr"
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
	SleepInterval Duration `json:"sleepInterval,omitempty"`
	// Time after which any request (keepers checks from sentinel etc...) will fail.
	RequestTimeout Duration `json:"requestTimeout,omitempty"`
	// Interval to wait for a db to be converged to the required state.
	ConvergenceTimeout Duration `json:"convergenceTimeout,omitempty"`
	// Interval to wait for a db to be initialized (doing a initdb)
	InitTimeout Duration `json:"initTimeout,omitempty"`
	// Interval after the first fail to declare a keeper or a db as not healthy.
	FailInterval Duration `json:"failInterval,omitempty"`
	// Max number of standbys. This needs to be greater enough to cover both
	// standby managed by stolon and additional standbys configured by the
	// user. Its value affect different postgres parameters like
	// max_replication_slots and max_wal_senders. Setting this to a number
	// lower than the sum of stolon managed standbys and user managed
	// standbys will have unpredicatable effects due to problems creating
	// replication slots or replication problems due to exhausted wal
	// senders.
	MaxStandbys uint16 `json:"maxStandbys,omitempty"`
	// Max number of standbys for every sender. A sender can be a master or
	// another standby (if/when implementing cascading replication).
	MaxStandbysPerSender uint16 `json:"maxStandbysPerSender,omitempty"`
	// Use Synchronous replication between master and its standbys
	SynchronousReplication bool `json:"synchronousReplication,omitempty"`
	// Whether to use pg_rewind
	UsePgrewind bool `json:"usePgrewind,omitempty"`
	// InitMode defines the cluster initialization mode. Current modes are: new, existing, pitr
	InitMode ClusterInitMode `json:"initMode,omitempty"`
	// Role defines the cluster operating role (master or standby of an external database)
	Role ClusterRole `json:"role,omitempty"`
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
	Role              ClusterRole  `json:"mode,omitempty"`
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
	return nc.(*Cluster)
}

func (s *ClusterSpec) SetDefaults() {
	if s.SleepInterval == ZeroDuration {
		s.SleepInterval = Duration{Duration: DefaultSleepInterval}
	}
	if s.RequestTimeout == ZeroDuration {
		s.RequestTimeout = Duration{Duration: DefaultRequestTimeout}
	}
	if s.ConvergenceTimeout == ZeroDuration {
		s.ConvergenceTimeout = Duration{Duration: DefaultConvergenceTimeout}
	}
	if s.InitTimeout == ZeroDuration {
		s.InitTimeout = Duration{Duration: DefaultInitTimeout}
	}
	if s.FailInterval == ZeroDuration {
		s.FailInterval = Duration{Duration: DefaultFailInterval}
	}
	if s.MaxStandbys == 0 {
		s.MaxStandbys = DefaultMaxStandbys
	}
	if s.MaxStandbysPerSender == 0 {
		s.MaxStandbysPerSender = DefaultMaxStandbysPerSender
	}
}

func (s *ClusterSpec) Validate() error {
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
	if s.FailInterval.Duration < 0 {
		return fmt.Errorf("failInterval must be positive")
	}
	if s.MaxStandbys < 1 {
		return fmt.Errorf("maxStandbys must be at least 1")
	}
	if s.MaxStandbysPerSender < 1 {
		return fmt.Errorf("maxStandbysPerSender must be at least 1")
	}
	if s.InitMode == "" {
		return fmt.Errorf("initMode undefined")
	}
	if s.InitMode == ClusterInitModeExisting {
		if s.ExistingConfig == nil {
			return fmt.Errorf("existingConfig undefined. Required when initMode is \"existing\"")
		}
		if s.ExistingConfig.KeeperUID == "" {
			return fmt.Errorf("existingConfig.keeperUID undefined")
		}
	}
	return nil
}

func (c *Cluster) UpdateSpec(ns *ClusterSpec) error {
	s := c.Spec
	ns.SetDefaults()
	if err := ns.Validate(); err != nil {
		return fmt.Errorf("invalid cluster spec: %v", err)
	}
	if s.InitMode != ns.InitMode {
		return fmt.Errorf("cannot change cluster init mode")
	}
	if s.Role == ClusterRoleMaster && ns.Role == ClusterRoleStandby {
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
	c.Spec.SetDefaults()
	return c
}

type KeeperSpec struct{}

type KeeperStatus struct {
	ErrorStartTime time.Time `json:"errorStartTime,omitempty"`
	Healthy        bool      `json:"healthy,omitempty"`

	ListenAddress string `json:"listenAddress,omitempty"`
	Port          string `json:"port,omitempty"`
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
			ErrorStartTime: time.Time{},
			Healthy:        true,
			ListenAddress:  ki.ListenAddress,
			Port:           ki.Port,
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
}

type DBStatus struct {
	ErrorStartTime time.Time `json:"errorStartTime,omitempty"`
	Healthy        bool      `json:"healthy,omitempty"`

	CurrentGeneration int64 `json:"currentGeneration,omitempty"`

	ListenAddress string `json:"listenAddress,omitempty"`
	Port          string `json:"port,omitempty"`

	SystemID         string                   `json:"systemdID,omitempty"`
	TimelineID       uint64                   `json:"timelineID,omitempty"`
	XLogPos          uint64                   `json:"xLogPos,omitempty"`
	TimelinesHistory PostgresTimelinesHistory `json:"timelinesHistory,omitempty"`
}

type DB struct {
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *DBSpec `json:"spec,omitempty"`

	Status DBStatus `json:"status,omitempty"`
}

type ProxySpec struct {
	MasterDBUID string `json:"masterDbUid,omitempty"`
}

type ProxyStatus struct {
	// TODO(sgotti) register current active proxies status. Useful
	// if in future we want to wait for all proxies having converged
	// before enabling new master
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

var ZeroDuration = Duration{}

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

func (k *Keeper) SetError() {
	if k.Status.ErrorStartTime.IsZero() {
		k.Status.ErrorStartTime = time.Now()
	}
}

func (k *Keeper) CleanError() {
	k.Status.ErrorStartTime = time.Time{}
}

func (d *DB) SetError() {
	if d.Status.ErrorStartTime.IsZero() {
		d.Status.ErrorStartTime = time.Now()
	}
}

func (d *DB) CleanError() {
	d.Status.ErrorStartTime = time.Time{}
}
