// Copyright 2018 Sorint.lab
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

	cmdcommon "github.com/sorintlab/stolon/cmd"
	"github.com/spf13/cobra"
)

var failKeeperCmd = &cobra.Command{
	Use:   "failkeeper [keeper uid]",
	Short: `Force keeper as "temporarily" failed. The sentinel will compute a new clusterdata considering it as failed and then restore its state to the real one.`,
	Long:  `Force keeper as "temporarily" failed. It's just a one shot operation, the sentinel will compute a new clusterdata considering the keeper as failed and then restore its state to the real one. For example, if the force failed keeper is a master, the sentinel will try to elect a new master. If no new master can be elected, the force failed keeper, if really healthy, will be re-elected as master`,
	Run:   failKeeper,
}

func init() {
	CmdStolonCtl.AddCommand(failKeeperCmd)
}

func failKeeper(cmd *cobra.Command, args []string) {
	if len(args) > 1 {
		die("too many arguments")
	}

	if len(args) == 0 {
		die("keeper uid required")
	}

	keeperID := args[0]

	store, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		die("%v", err)
	}

	cd, pair, err := getClusterData(store)
	if err != nil {
		die("cannot get cluster data: %v", err)
	}
	if cd.Cluster == nil {
		die("no cluster spec available")
	}
	if cd.Cluster.Spec == nil {
		die("no cluster spec available")
	}

	newCd := cd.DeepCopy()
	keeperInfo := newCd.Keepers[keeperID]
	if keeperInfo == nil {
		die("keeper doesn't exist")
	}

	keeperInfo.Status.ForceFail = true

	_, err = store.AtomicPutClusterData(context.TODO(), newCd, pair)
	if err != nil {
		die("cannot update cluster data: %v", err)
	}
}
