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
	"context"

	cmdcommon "github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/spf13/cobra"
)

var removeKeeperCmd = &cobra.Command{
	Use:   "removekeeper [keeper uid]",
	Short: "Removes keeper from cluster data",
	Run:   removeKeeper,
}

func init() {
	CmdStolonCtl.AddCommand(removeKeeperCmd)
}

func removeKeeper(cmd *cobra.Command, args []string) {
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

	keeperDb := getDbForKeeper(newCd.DBs, keeperID)

	if keeperDb != nil {
		if newCd.Cluster.Status.Master == keeperDb.UID {
			die("keeper assigned db is the current cluster master db.")
		}
	}

	delete(newCd.Keepers, keeperID)
	if keeperDb != nil {
		delete(newCd.DBs, keeperDb.UID)
	}

	// NOTE: if the removed db is listed inside another db.Followers it'll will
	// be cleaned by the sentinels

	_, err = store.AtomicPutClusterData(context.TODO(), newCd, pair)
	if err != nil {
		die("cannot update cluster data: %v", err)
	}
}

func getDbForKeeper(dbs cluster.DBs, keeperID string) *cluster.DB {
	for _, db := range dbs {
		if db.Spec.KeeperUID == keeperID {
			return db
		}
	}

	return nil
}
