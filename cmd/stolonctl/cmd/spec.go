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

package cmd

import (
	"encoding/json"

	cmdcommon "github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/internal/cluster"

	"github.com/spf13/cobra"
)

var cmdSpec = &cobra.Command{
	Use:   "spec",
	Run:   spec,
	Short: "Retrieve the current cluster specification",
}

type specOptions struct {
	defaults bool
}

var specOpts specOptions

func init() {
	cmdSpec.PersistentFlags().BoolVar(&specOpts.defaults, "defaults", false, "also show default values")

	CmdStolonCtl.AddCommand(cmdSpec)
}

type ClusterSpecNoDefaults struct {
	SleepInterval                    *cluster.Duration         `json:"sleepInterval,omitempty"`
	RequestTimeout                   *cluster.Duration         `json:"requestTimeout,omitempty"`
	ConvergenceTimeout               *cluster.Duration         `json:"convergenceTimeout,omitempty"`
	InitTimeout                      *cluster.Duration         `json:"initTimeout,omitempty"`
	SyncTimeout                      *cluster.Duration         `json:"syncTimeout,omitempty"`
	DBWaitReadyTimeout               *cluster.Duration         `json:"dbWaitReadyTimeout,omitempty"`
	FailInterval                     *cluster.Duration         `json:"failInterval,omitempty"`
	DeadKeeperRemovalInterval        *cluster.Duration         `json:"deadKeeperRemovalInterval,omitempty"`
	ProxyCheckInterval               *cluster.Duration         `json:"proxyCheckInterval,omitempty"`
	ProxyTimeout                     *cluster.Duration         `json:"proxyTimeout,omitempty"`
	MaxStandbys                      *uint16                   `json:"maxStandbys,omitempty"`
	MaxStandbysPerSender             *uint16                   `json:"maxStandbysPerSender,omitempty"`
	MaxStandbyLag                    *uint32                   `json:"maxStandbyLag,omitempty"`
	SynchronousReplication           *bool                     `json:"synchronousReplication,omitempty"`
	MinSynchronousStandbys           *uint16                   `json:"minSynchronousStandbys,omitempty"`
	MaxSynchronousStandbys           *uint16                   `json:"maxSynchronousStandbys,omitempty"`
	AdditionalWalSenders             *uint16                   `json:"additionalWalSenders,omitempty"`
	AdditionalMasterReplicationSlots []string                  `json:"additionalMasterReplicationSlots,omitempty"`
	UsePgrewind                      *bool                     `json:"usePgrewind,omitempty"`
	InitMode                         *cluster.ClusterInitMode  `json:"initMode,omitempty"`
	MergePgParameters                *bool                     `json:"mergePgParameters,omitempty"`
	Role                             *cluster.ClusterRole      `json:"role,omitempty"`
	NewConfig                        *cluster.NewConfig        `json:"newConfig,omitempty"`
	PITRConfig                       *cluster.PITRConfig       `json:"pitrConfig,omitempty"`
	ExistingConfig                   *cluster.ExistingConfig   `json:"existingConfig,omitempty"`
	StandbyConfig                    *cluster.StandbyConfig    `json:"standbyConfig,omitempty"`
	DefaultSUReplAccessMode          *cluster.SUReplAccessMode `json:"defaultSUReplAccessMode,omitempty"`
	PGParameters                     cluster.PGParameters      `json:"pgParameters,omitempty"`
	PGHBA                            []string                  `json:"pgHBA,omitempty"`
	AutomaticPgRestart               *bool                     `json:"automaticPgRestart,omitempty"`
}

type ClusterSpecDefaults struct {
	SleepInterval                    *cluster.Duration         `json:"sleepInterval"`
	RequestTimeout                   *cluster.Duration         `json:"requestTimeout"`
	ConvergenceTimeout               *cluster.Duration         `json:"convergenceTimeout"`
	InitTimeout                      *cluster.Duration         `json:"initTimeout"`
	SyncTimeout                      *cluster.Duration         `json:"syncTimeout"`
	DBWaitReadyTimeout               *cluster.Duration         `json:"dbWaitReadyTimeout"`
	FailInterval                     *cluster.Duration         `json:"failInterval"`
	DeadKeeperRemovalInterval        *cluster.Duration         `json:"deadKeeperRemovalInterval"`
	ProxyCheckInterval               *cluster.Duration         `json:"proxyCheckInterval"`
	ProxyTimeout                     *cluster.Duration         `json:"proxyTimeout"`
	MaxStandbys                      *uint16                   `json:"maxStandbys"`
	MaxStandbysPerSender             *uint16                   `json:"maxStandbysPerSender"`
	MaxStandbyLag                    *uint32                   `json:"maxStandbyLag"`
	SynchronousReplication           *bool                     `json:"synchronousReplication"`
	MinSynchronousStandbys           *uint16                   `json:"minSynchronousStandbys"`
	MaxSynchronousStandbys           *uint16                   `json:"maxSynchronousStandbys"`
	AdditionalWalSenders             *uint16                   `json:"additionalWalSenders"`
	AdditionalMasterReplicationSlots []string                  `json:"additionalMasterReplicationSlots"`
	UsePgrewind                      *bool                     `json:"usePgrewind"`
	InitMode                         *cluster.ClusterInitMode  `json:"initMode"`
	MergePgParameters                *bool                     `json:"mergePgParameters"`
	Role                             *cluster.ClusterRole      `json:"role"`
	NewConfig                        *cluster.NewConfig        `json:"newConfig"`
	PITRConfig                       *cluster.PITRConfig       `json:"pitrConfig"`
	ExistingConfig                   *cluster.ExistingConfig   `json:"existingConfig"`
	StandbyConfig                    *cluster.StandbyConfig    `json:"standbyConfig"`
	DefaultSUReplAccessMode          *cluster.SUReplAccessMode `json:"defaultSUReplAccessMode"`
	PGParameters                     cluster.PGParameters      `json:"pgParameters"`
	PGHBA                            []string                  `json:"pgHBA"`
	AutomaticPgRestart               *bool                     `json:"automaticPgRestart"`
}

func spec(cmd *cobra.Command, args []string) {
	e, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		die("%v", err)
	}

	cd, _, err := getClusterData(e)
	if err != nil {
		die("%v", err)
	}
	if cd.Cluster == nil {
		die("no cluster spec available")
	}
	if cd.Cluster.Spec == nil {
		die("no cluster spec available")
	}

	var specj []byte
	if specOpts.defaults {
		cs := (*ClusterSpecDefaults)(cd.Cluster.DefSpec())
		specj, err = json.MarshalIndent(cs, "", "\t")
	} else {
		cs := (*ClusterSpecNoDefaults)(cd.Cluster.Spec)
		specj, err = json.MarshalIndent(cs, "", "\t")
	}
	if err != nil {
		die("failed to marshall spec: %v", err)
	}

	stdout("%s", specj)
}
