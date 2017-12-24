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
	"sort"
	"text/tabwriter"

	"github.com/sorintlab/stolon/pkg/cluster"

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
	// skip not existing db: specified as a follower but not available in the
	// clister spec (this should happen only when doing a stolonctl
	// removekeeper)
	if _, ok := cd.DBs[dbuid]; !ok {
		return
	}
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

	e, err := NewStore()
	if err != nil {
		die("cannot create store: %v", err)
	}

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

	cd, _, err := getClusterData(e)
	if err != nil {
		die("%v", err)
	}

	stdout("")
	stdout("=== Keepers ===")
	stdout("")
	if cd.Keepers == nil {
		stdout("No keepers available")
		stdout("")
	} else {
		kssKeys := cd.Keepers.SortedKeys()
		fmt.Fprintf(tabOut, "UID\tHEALTHY\tPG LISTENADDRESS\tPG HEALTHY\tPG WANTEDGENERATION\tPG CURRENTGENERATION\n")
		for _, kuid := range kssKeys {
			k := cd.Keepers[kuid]
			db := cd.FindDB(k)
			if db != nil {
				dbListenAddress := "(unknown)"
				if db.Status.ListenAddress != "" {
					dbListenAddress = fmt.Sprintf("%s:%s", db.Status.ListenAddress, db.Status.Port)
				}
				fmt.Fprintf(tabOut, "%s\t%t\t%s\t%t\t%d\t%d\t\n", k.UID, k.Status.Healthy, dbListenAddress, db.Status.Healthy, db.Generation, db.Status.CurrentGeneration)
			} else {
				fmt.Fprintf(tabOut, "%s\t%t\t(no db assigned)\t\t\t\t\n", k.UID, k.Status.Healthy)
			}
		}
	}
	tabOut.Flush()

	if cd.Cluster == nil || cd.DBs == nil {
		stdout("No cluster available")
		return
	}

	master := cd.Cluster.Status.Master
	stdout("")
	stdout("=== Cluster Info ===")
	stdout("")
	if master != "" {
		stdout("Master: %s", cd.Keepers[cd.DBs[master].Spec.KeeperUID].UID)
	} else {
		stdout("Master Keeper: (none)")
	}

	if master != "" {
		stdout("")
		stdout("===== Keepers/DB tree =====")
		masterDB := cd.DBs[master]
		stdout("")
		printTree(masterDB.UID, cd, 0, "", true)
	}

	stdout("")
}
