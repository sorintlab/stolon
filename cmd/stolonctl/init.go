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

package main

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/spf13/cobra"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Run:   initCluster,
	Short: "Initialize a new cluster",
}

type InitOptions struct {
	file     string
	forceYes bool
}

var initOpts InitOptions

func init() {
	cmdInit.PersistentFlags().StringVarP(&initOpts.file, "file", "f", "", "file contaning the new cluster spec")
	cmdInit.PersistentFlags().BoolVarP(&initOpts.forceYes, "yes", "y", false, "don't ask for confirmation")

	cmdStolonCtl.AddCommand(cmdInit)
}

func initCluster(cmd *cobra.Command, args []string) {
	if len(args) > 1 {
		die("too many arguments")
	}

	data := []byte{}
	switch len(args) {
	case 1:
		data = []byte(args[0])
	case 0:
		if initOpts.file != "" {
			var err error
			if initOpts.file == "-" {
				data, err = ioutil.ReadAll(os.Stdin)
				if err != nil {
					die("cannot read from stdin: %v", err)
				}
			} else {
				data, err = ioutil.ReadFile(initOpts.file)
				if err != nil {
					die("cannot read file: %v", err)
				}
			}
		}
	}

	e, err := NewStore()
	if err != nil {
		die("cannot create store: %v", err)
	}

	cd, _, err := e.GetClusterData()
	if err != nil {
		die("cannot get cluster data: %v", err)
	}
	if cd != nil {
		stdout("WARNING: The current cluster data will be removed")
	}
	stdout("WARNING: The databases managed by the keepers will be overwritten depending on the provided cluster spec.")

	accepted := true
	if !initOpts.forceYes {
		accepted, err = askConfirmation("Are you sure you want to continue? [yes/no] ")
		if err != nil {
			die("%v", err)
		}
	}
	if !accepted {
		stdout("exiting")
		os.Exit(0)
	}

	cd, _, err = e.GetClusterData()
	if err != nil {
		die("cannot get cluster data: %v", err)
	}

	var cs *cluster.ClusterSpec
	if len(data) == 0 {
		// Define a new cluster spec with initMode "new"
		cs = &cluster.ClusterSpec{}
		cs.InitMode = cluster.ClusterInitModeP(cluster.ClusterInitModeNew)
	} else {
		if err := json.Unmarshal(data, &cs); err != nil {
			die("failed to unmarshal cluster spec: %v", err)
		}
	}

	if err := cs.Validate(); err != nil {
		die("invalid cluster spec: %v", err)
	}

	c := cluster.NewCluster(common.UID(), cs)
	cd = cluster.NewClusterData(c)

	// We ignore if cd has been modified between reading and writing
	if err := e.PutClusterData(cd); err != nil {
		die("cannot update cluster data: %v", err)
	}
}
