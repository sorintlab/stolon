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
	"github.com/sorintlab/stolon/internal/cluster"
	slog "github.com/sorintlab/stolon/internal/log"
	"github.com/sorintlab/stolon/internal/store"
	"github.com/spf13/cobra"
)

var log = slog.S()

var pgRestartCmd = &cobra.Command{
	Use:   "pgrestart",
	Short: `Restarts the underlying postgres cluster with minimal downtime`,
	Long:  `Restarts the underlying postgres cluster with minimal downtime`,
	Run:   pgRestart,
}

func init() {
	CmdStolonCtl.AddCommand(pgRestartCmd)
}

func pgRestart(cmd *cobra.Command, args []string) {
	store, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		die("%v", err)
	}

	cd, pair := fatalGetClusterData(store)

	newCd := cd.DeepCopy()

	log.Infow("Restarting all postgres instances!")
	for _, db := range newCd.DBs {
		k, ok := newCd.Keepers[db.Spec.KeeperUID]

		if !ok {
			die("Keeper data was empty for the DB")
		}

		k.Status.Healthy = false
		k.Status.SchedulePgRestart = true
	}

	_, err = store.AtomicPutClusterData(context.TODO(), newCd, pair)
	if err != nil {
		die("cannot update cluster data: %v", err)
	}
}

func fatalGetClusterData(e store.Store) (*cluster.ClusterData, *store.KVPair) {
	cd, pair, err := getClusterData(e)
	if err != nil {
		die("cannot get cluster data: %v", err)
	}
	if cd.Cluster == nil {
		die("no cluster spec available")
	}
	if cd.Cluster.Spec == nil {
		die("no cluster spec available")
	}

	return cd, pair
}
