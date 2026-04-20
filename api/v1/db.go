// Copyright 2026 PgVillage
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

package v1

import (
	"time"

	"github.com/pgvillage-tools/stolon/internal/common"
)

// DBSpec defines the spec of a database
type DBSpec struct {
	// The KeeperUID this db is assigned to
	KeeperUID string `json:"keeperUID,omitempty"`
	// Time after which any request (keepers checks from sentinel etc...) will fail.
	RequestTimeout Duration `json:"requestTimeout,omitempty"`
	// See Spec MaxStandbys description
	MaxStandbys uint16 `json:"maxStandbys,omitempty"`
	// Use Synchronous replication between master and its standbys
	SynchronousReplication bool `json:"synchronousReplication,omitempty"`
	// Whether to use pg_rewind
	UsePgrewind bool `json:"usePgrewind,omitempty"`
	// AdditionalWalSenders defines the number of additional wal_senders in
	// addition to the ones internally defined by stolon
	AdditionalWalSenders uint16 `json:"additionalWalSenders"`
	// AdditionalReplicationSlots is a list of additional replication slots.
	// Replication slots not defined here will be dropped from the instance
	// (i.e. manually created replication slots will be removed).
	AdditionalReplicationSlots []string `json:"additionalReplicationSlots"`
	// InitMode defines the db initialization mode. Current modes are: none, new
	InitMode DBInitMode `json:"initMode,omitempty"`
	// Init configuration used when InitMode is "new"
	NewConfig *NewConfig `json:"newConfig,omitempty"`
	// Point in time recovery init configuration used when InitMode is "pitr"
	PITRConfig *PITRConfig `json:"pitrConfig,omitempty"`
	// Map of postgres parameters
	PGParameters PGParameters `json:"pgParameters,omitempty"`
	// Additional pg_hba.conf entries
	// We don't set omitempty since we want to distinguish between null or empty slice
	PGHBA []string `json:"pgHBA"`
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
	// External SynchronousStandbys are external standbys names to be configured as synchronous
	ExternalSynchronousStandbys []string `json:"externalSynchronousStandbys"`
}

// DBStatus defines the status of a DB
type DBStatus struct {
	Healthy bool `json:"healthy,omitempty"`

	CurrentGeneration int64 `json:"currentGeneration,omitempty"`

	ListenAddress string `json:"listenAddress,omitempty"`
	Port          string `json:"port,omitempty"`

	SystemID         string                   `json:"systemdID,omitempty"`
	TimelineID       uint64                   `json:"timelineID,omitempty"`
	XLogPos          uint64                   `json:"xLogPos,omitempty"`
	TimelinesHistory PostgresTimelinesHistory `json:"timelinesHistory,omitempty"`

	PGParameters PGParameters `json:"pgParameters,omitempty"`

	// DBUIDs of the internal standbys currently reported as in sync by the instance
	CurSynchronousStandbys []string `json:"-"`

	// DBUIDs of the internal standbys that we know are in sync.
	// They could be currently down but we know that they were reported as in
	// sync in the past and they are defined inside synchronous_standby_names
	// so the instance will wait for acknowledge from them.
	SynchronousStandbys []string `json:"synchronousStandbys"`

	// NOTE(sgotti) we currently don't report the external synchronous standbys.
	// If/when needed lets add a new ExternalSynchronousStandbys field

	OlderWalFile string `json:"olderWalFile,omitempty"`
}

// DB is an instance in a cluster
type DB struct {
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *DBSpec `json:"spec,omitempty"`

	Status DBStatus `json:"status,omitempty"`
}

// DBs can store all databases for a cluster
type DBs map[string]*DB
