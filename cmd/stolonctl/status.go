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
	Use: "status",
	Run: status,
}

func init() {
	cmdStolonCtl.AddCommand(cmdStatus)
}

func printTree(id string, cv *cluster.ClusterView, level int, prefix string, tail bool) {
	out := prefix
	if level > 0 {
		if tail {
			out += "└─"
		} else {
			out += "├─"
		}
	}
	out += id
	if id == cv.Master {
		out += " (master)"
	}
	stdout(out)
	followersIDs := cv.GetFollowersIDs(id)
	c := len(followersIDs)
	for i, f := range cv.GetFollowersIDs(id) {
		emptyspace := ""
		if level > 0 {
			emptyspace = "  "
		}
		linespace := "│ "
		if i < c-1 {
			if tail {
				printTree(f, cv, level+1, prefix+emptyspace, false)
			} else {
				printTree(f, cv, level+1, prefix+linespace, false)
			}
		} else {
			if tail {
				printTree(f, cv, level+1, prefix+emptyspace, true)
			} else {
				printTree(f, cv, level+1, prefix+linespace, true)
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
		fmt.Fprintf(tabOut, "ID\tLISTENADDRESS\tLEADER\n")
		for _, si := range sentinelsInfo {
			leader := false
			if lsid != "" {
				if si.ID == lsid {
					leader = true
				}
			}
			fmt.Fprintf(tabOut, "%s\t%s:%s\t%t\n", si.ID, si.ListenAddress, si.Port, leader)
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
		fmt.Fprintf(tabOut, "ID\tLISTENADDRESS\tCV VERSION\n")
		for _, pi := range proxiesInfo {
			fmt.Fprintf(tabOut, "%s\t%s:%s\t%d\n", pi.ID, pi.ListenAddress, pi.Port, pi.ClusterViewVersion)
			tabOut.Flush()
		}
	}

	clusterData, _, err := e.GetClusterData()
	if err != nil {
		die("cannot get cluster data: %v", err)
	}
	if clusterData == nil {
		die("cluster data not available: %v", err)
	}
	cv := clusterData.ClusterView
	kss := clusterData.KeepersState

	stdout("")
	stdout("=== Keepers ===")
	stdout("")
	if kss == nil {
		stdout("No keepers state available")
		stdout("")
	} else {
		kssKeys := kss.SortedKeys()
		fmt.Fprintf(tabOut, "ID\tLISTENADDRESS\tPG LISTENADDRESS\tCV VERSION\tHEALTHY\n")
		for _, k := range kssKeys {
			ks := kss[k]
			fmt.Fprintf(tabOut, "%s\t%s:%s\t%s:%s\t%d\t%t\n", ks.ID, ks.ListenAddress, ks.Port, ks.PGListenAddress, ks.PGPort, ks.ClusterViewVersion, ks.Healthy)
		}
	}
	tabOut.Flush()

	stdout("")
	stdout("=== Required Cluster View ===")
	stdout("")
	stdout("Version: %d", cv.Version)
	if cv == nil {
		stdout("No clusterview available")
	} else {
		stdout("Master: %s", cv.Master)
		stdout("")
		stdout("===== Keepers tree =====")
		for _, mr := range cv.KeepersRole {
			if mr.Follow == "" {
				stdout("")
				printTree(mr.ID, cv, 0, "", true)
			}
		}
	}

	stdout("")
}
