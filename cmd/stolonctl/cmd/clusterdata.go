// Copyright 2017 Sorint.lab
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

	"github.com/spf13/cobra"
)

var cmdClusterData = &cobra.Command{
	Use:   "clusterdata",
	Run:   clusterdata,
	Short: "Retrieve the current cluster data",
}

type clusterdataOptions struct {
	pretty bool
}

var clusterdataOpts clusterdataOptions

func init() {
	cmdClusterData.PersistentFlags().BoolVar(&clusterdataOpts.pretty, "pretty", false, "pretty print")

	CmdStolonCtl.AddCommand(cmdClusterData)
}

func clusterdata(cmd *cobra.Command, args []string) {
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
		die("no cluster clusterdata available")
	}
	var clusterdataj []byte
	if clusterdataOpts.pretty {
		clusterdataj, err = json.MarshalIndent(cd, "", "\t")
		if err != nil {
			die("failed to marshall clusterdata: %v", err)
		}
	} else {
		clusterdataj, err = json.Marshal(cd)
		if err != nil {
			die("failed to marshall clusterdata: %v", err)
		}
	}
	stdout("%s", clusterdataj)
}
