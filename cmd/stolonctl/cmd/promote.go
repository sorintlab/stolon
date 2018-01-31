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

package cmd

import (
	"context"
	"os"

	cmdcommon "github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/store"

	"github.com/spf13/cobra"
)

var cmdPromote = &cobra.Command{
	Use:   "promote",
	Run:   promote,
	Short: "Promotes a standby cluster to a primary cluster",
}

func init() {
	cmdPromote.PersistentFlags().BoolVarP(&initOpts.forceYes, "yes", "y", false, "don't ask for confirmation")

	CmdStolonCtl.AddCommand(cmdPromote)
}

func promote(cmd *cobra.Command, args []string) {
	if len(args) > 0 {
		die("too many arguments")
	}

	e, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		die("%v", err)
	}

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

	retry := 0
	for retry < maxRetries {
		cd, pair, err := getClusterData(e)
		if err != nil {
			die("%v", err)
		}
		if cd.Cluster == nil {
			die("no cluster spec available")
		}
		if cd.Cluster.Spec == nil {
			die("no cluster spec available")
		}

		ds := cd.Cluster.DefSpec()
		if *ds.Role == cluster.ClusterRoleMaster {
			stderr("cluster spec role already set to master")
			os.Exit(0)
		}
		cd.Cluster.Spec.Role = cluster.ClusterRoleP(cluster.ClusterRoleMaster)

		if err := cd.Cluster.UpdateSpec(cd.Cluster.Spec); err != nil {
			die("Cannot update cluster spec: %v", err)
		}

		// retry if cd has been modified between reading and writing
		_, err = e.AtomicPutClusterData(context.TODO(), cd, pair)
		if err != nil {
			if err == store.ErrKeyModified {
				retry++
				continue
			}
			die("cannot update cluster data: %v", err)
		}
		break
	}
	if retry == maxRetries {
		die("failed to update cluster data after %d retries", maxRetries)
	}
}
