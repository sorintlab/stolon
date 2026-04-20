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

// FollowType is an enum for following internal (local primary keeper) or external (other cluster)
type FollowType string

const (
	// FollowTypeInternal specifies that a db managed by a keeper in our cluster is followed
	FollowTypeInternal FollowType = "internal"
	// FollowTypeExternal specifies that a db in another cluster is followed
	FollowTypeExternal FollowType = "external"
)

// FollowConfig specifies the config for this keeper to follow another instance
type FollowConfig struct {
	Type FollowType `json:"type,omitempty"`
	// Keeper ID to follow when Type is "internal"
	DBUID string `json:"dbuid,omitempty"`
	// Standby settings when Type is "external"
	StandbySettings         *StandbySettings         `json:"standbySettings,omitempty"`
	ArchiveRecoverySettings *ArchiveRecoverySettings `json:"archiveRecoverySettings,omitempty"`
}

// Phase is an enum for the phase that a cluster is in
type Phase string

const (
	// Initializing phase means the cluster is initializing
	Initializing Phase = "initializing"
	// Normal phase means the cluster is initialized and ready to be used
	Normal Phase = "normal"
)

// Role defines the role that a Keeper has
type Role string

const (
	// Primary means that bthe instance is promoted and available for read/write
	Primary Role = "master"
	// Replica means that the instance is replicating changes for another upstream replica or Primary
	Replica Role = "standby"
)

// InitMode is an enum for the init mode that a Cluster is in
type InitMode string

const (
	// New initializes a cluster starting from a freshly initialized database cluster. Valid only when cluster role is
	// master.
	New InitMode = "new"
	// PITR initializes a cluster doing a point in time recovery on a keeper.
	PITR InitMode = "pitr"
	// ExistingCluster reuses an already Initialized cluster
	ExistingCluster InitMode = "existing"
)

// DBInitMode is an enum for the init mode that a database is in
type DBInitMode string

const (
	// NoDB does not initialize a database
	NoDB DBInitMode = "none"
	// ExistingDB reuses the existing db
	ExistingDB DBInitMode = "existing"
	// NewDB initializes a new database
	NewDB DBInitMode = "new"
	// PITRDB initialized a database using Point in time Recovery
	PITRDB DBInitMode = "pitr"
	// ResyncDB syncs a database from another source
	ResyncDB DBInitMode = "resync"
)
