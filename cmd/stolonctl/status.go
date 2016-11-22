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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/store"

	"github.com/spf13/cobra"
)

var cmdStatus = &cobra.Command{
	Use:   "status",
	Run:   status,
	Short: "Display the current cluster status",
}

func init() {
	cmdStolonCtl.AddCommand(cmdStatus)
}

func printTree(dbuid string, cd *cluster.ClusterData, level int, prefix string, tail bool) {
	out := prefix
	if level > 0 {
		if tail {
			out += "└─"
		} else {
			out += "├─"
		}
	}
	out += cd.DBs[dbuid].Spec.KeeperUID
	if dbuid == cd.Cluster.Status.Master {
		out += " (master)"
	}
	stdout(out)
	db := cd.DBs[dbuid]
	followers := db.Spec.Followers
	c := len(followers)
	for i, f := range followers {
		emptyspace := ""
		if level > 0 {
			emptyspace = "  "
		}
		linespace := "│ "
		if i < c-1 {
			if tail {
				printTree(f, cd, level+1, prefix+emptyspace, false)
			} else {
				printTree(f, cd, level+1, prefix+linespace, false)
			}
		} else {
			if tail {
				printTree(f, cd, level+1, prefix+emptyspace, true)
			} else {
				printTree(f, cd, level+1, prefix+linespace, true)
			}
		}
	}
}

func status(cmd *cobra.Command, args []string) {
	tabOut := new(tabwriter.Writer)
	tabOut.Init(os.Stdout, 0, 8, 1, '\t', 0)

	if cfg.clusterName == "" {
		die("cluster name required")
	}
	storePath := filepath.Join(common.StoreBasePath, cfg.clusterName)

	kvstore, err := store.NewStore(store.Backend(cfg.storeBackend), cfg.storeEndpoints)
	if err != nil {
		die("cannot create store: %v", err)
	}
	e := store.NewStoreManager(kvstore, storePath)

	sentinelsInfo, err := e.GetSentinelsInfo()
	if err != nil {
		die("cannot get sentinels info: %v", err)
	}

	lsid, err := e.GetLeaderSentinelId()
	if err != nil {
		die("cannot get leader sentinel info")
	}

	stdout("=== Active sentinels ===")
	stdout("")
	if len(sentinelsInfo) == 0 {
		stdout("No active sentinels")
	} else {
		sort.Sort(sentinelsInfo)
		fmt.Fprintf(tabOut, "ID\tLEADER\n")
		for _, si := range sentinelsInfo {
			leader := false
			if lsid != "" {
				if si.UID == lsid {
					leader = true
				}
			}
			fmt.Fprintf(tabOut, "%s\t%t\n", si.UID, leader)
			tabOut.Flush()
		}
	}

	proxiesInfo, err := e.GetProxiesInfo()
	if err != nil {
		die("cannot get proxies info: %v", err)
	}

	stdout("")
	stdout("=== Active proxies ===")
	stdout("")
	if len(proxiesInfo) == 0 {
		stdout("No active proxies")
	} else {
		sort.Sort(proxiesInfo)
		fmt.Fprintf(tabOut, "ID\n")
		for _, pi := range proxiesInfo {
			fmt.Fprintf(tabOut, "%s\n", pi.UID)
			tabOut.Flush()
		}
	}

	cd, _, err := e.GetClusterData()
	if err != nil {
		die("cannot get cluster data: %v", err)
	}
	if cd == nil {
		die("cluster data not available: %v", err)
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		die("unsupported cluster data format version %d", cd.FormatVersion)
	}

	stdout("")
	stdout("=== Keepers ===")
	stdout("")
	if cd.Keepers == nil {
		stdout("No keepers available")
		stdout("")
	} else {
		kssKeys := cd.Keepers.SortedKeys()
		fmt.Fprintf(tabOut, "UID\t\tPG LISTENADDRESS\tHEALTHY\tPGWANTEDGENERATION\tPGCURRENTGENERATION\n")
		for _, kuid := range kssKeys {
			k := cd.Keepers[kuid]
			db := cd.FindDB(k)
			fmt.Fprintf(tabOut, "%s\t%s:%s\t%t\t%d\t%d\n", k.UID, db.Status.ListenAddress, db.Status.Port, k.Status.Healthy, db.Generation, db.Status.CurrentGeneration)
		}
	}
	tabOut.Flush()

	stdout("")
	stdout("=== Required Cluster ===")
	stdout("")
	if cd.Cluster == nil || cd.DBs == nil {
		stdout("No cluster available")
		return
	}
	stdout("Master: %s", cd.Keepers[cd.DBs[cd.Cluster.Status.Master].Spec.KeeperUID].UID)
	stdout("")
	stdout("===== Keepers tree =====")
	for _, db := range cd.DBs {
		if db.Spec.Role == common.RoleMaster {
			stdout("")
			printTree(db.UID, cd, 0, "", true)
		}
	}

	stdout("")
}
