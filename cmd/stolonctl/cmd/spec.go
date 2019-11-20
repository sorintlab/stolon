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
	cs := cd.Cluster.Spec
	if specOpts.defaults {
		cs = cd.Cluster.DefSpec()
	}
	specj, err := json.MarshalIndent(cluster.ClusterSpecNew(*cs), "", "\t")
	if err != nil {
		die("failed to marshall spec: %v", err)
	}

	stdout("%s", specj)
}
