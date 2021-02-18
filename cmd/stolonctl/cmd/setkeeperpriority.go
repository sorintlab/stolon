// Copyright 2019 Sorint.lab
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
	"strconv"

	cmdcommon "github.com/sorintlab/stolon/cmd"
	"github.com/spf13/cobra"
)

var setKeeperPriorityCmd = &cobra.Command{
	Use:   "setkeeperpriority [keeper uid] [priority]",
	Short: `Set priority of keeper.`,
	Long: `Stolon will promote available keeper with higher priority than current master,
if this is possible. This is the same as --priority keeper option, but allows to
change priority without restarting the keeper (along with underlying Postgres).`,
	Run:  setKeeperPriority,
	Args: cobra.ExactArgs(2), // exactly 2 positional args
}

func init() {
	CmdStolonCtl.AddCommand(setKeeperPriorityCmd)
}

func setKeeperPriority(cmd *cobra.Command, args []string) {
	keeperID := args[0]
	priority, err := strconv.Atoi(args[1])
	if err != nil {
		die("priority must be integer: %v", err)
	}

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
	keeper := newCd.Keepers[keeperID]
	if keeper == nil {
		die("keeper doesn't exist")
	}

	keeper.Spec.Priority = priority

	_, err = store.AtomicPutClusterData(context.TODO(), newCd, pair)
	if err != nil {
		die("cannot update cluster data: %v", err)
	}
}
