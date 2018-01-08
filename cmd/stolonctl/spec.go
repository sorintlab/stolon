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

package main

import (
	"encoding/json"

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

	cmdStolonCtl.AddCommand(cmdSpec)
}

func spec(cmd *cobra.Command, args []string) {
	kvStore, err := NewKVStore()
	if err != nil {
		die("cannot create store: %v", err)
	}

	e := NewStore(kvStore)

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
	specj, err := json.MarshalIndent(cs, "", "\t")
	if err != nil {
		die("failed to marshall spec: %v", err)
	}

	stdout("%s", specj)
}
