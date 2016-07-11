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
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/gravitational/stolon/pkg/cluster"
)

func status(client *client, clusterName string, masterOnly string) error {
	clt, err := client.getCluster(clusterName)
	if err != nil {
		return trace.Wrap(err)
	}
	if masterOnly {
		return masterStatus(cluster)
	}

	tabOut := new(tabwriter.Writer)
	tabOut.Init(os.Fmt.Println, 0, 8, 1, '\t', 0)

	sentinelsInfo, err := clt.GetSentinelsInfo()
	if err != nil {
		return trace.Wrap("cannot get sentinels info: %v", err)
	}

	lsid, err := clt.GetLeaderSentinelId()
	if err != nil {
		return trace.Wrap("cannot get leader sentinel info")
	}

	fmt.Println("Active sentinels")
	if len(sentinelsInfo) == 0 {
		fmt.Println("No active sentinels")
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

	proxiesInfo, err := clt.GetProxiesInfo()
	if err != nil {
		return trace.Wrap("cannot get proxies info: %v", err)
	}

	fmt.Println("=== Active proxies ===")
	if len(proxiesInfo) == 0 {
		fmt.Println("No active proxies")
	} else {
		sort.Sort(proxiesInfo)
		fmt.Fprintf(tabOut, "ID\tLISTENADDRESS\tCV VERSION\n")
		for _, pi := range proxiesInfo {
			fmt.Fprintf(tabOut, "%s\t%s:%s\t%d\n", pi.ID, pi.ListenAddress, pi.Port, pi.ClusterViewVersion)
			tabOut.Flush()
		}
	}

	clusterData, _, err := clt.GetClusterData()
	if err != nil {
		return trace.Wrap("cannot get cluster data: %v", err)
	}
	if clusterData == nil {
		return trace.Wrap("cluster data not available: %v", err)
	}
	cv := clusterData.ClusterView
	kss := clusterData.KeepersState

	fmt.Println("Keepers")
	if kss == nil {
		fmt.Println("No keepers state available")
	} else {
		kssKeys := kss.SortedKeys()
		fmt.Fprintf(tabOut, "ID\tLISTENADDRESS\tPG LISTENADDRESS\tCV VERSION\tHEALTHY\n")
		for _, k := range kssKeys {
			ks := kss[k]
			fmt.Fprintf(tabOut, "%s\t%s:%s\t%s:%s\t%d\t%t\n", ks.ID, ks.ListenAddress, ks.Port, ks.PGListenAddress, ks.PGPort, ks.ClusterViewVersion, ks.Healthy)
		}
	}
	tabOut.Flush()

	fmt.Println("Required Cluster View")
	fmt.Printf("Version: %d", cv.Version)
	if cv == nil {
		fmt.Println("No clusterview available")
	} else {
		fmt.Printf("Master: %s\n", cv.Master)
		fmt.Println("Keepers tree")
		for _, mr := range cv.KeepersRole {
			if mr.Follow == "" {
				printTree(mr.ID, cv, 0, "", true)
			}
		}
	}

	fmt.Println("")
}

func masterStatus(clt *clusterClient) error {
	clusterData, _, err := clt.GetClusterData()
	if err != nil {
		return trace.Wrap(err, "cannot get cluster data")
	}
	if clusterData == nil {
		return trace.NotFound("cluster data not available")
	}
	cv := clusterData.ClusterView
	kss := clusterData.KeepersState
	masterData := kss[cv.Master]
	if outputFormat == OutputJSON {
		data, err := json.Marshal(masterData)
		if err != nil {
			return trace.Wrap(err, "can't convert to json")
		}
		fmt.Println(string(data))
	} else {
		fmt.Println(masterData)
	}
	return nil
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
	fmt.Println(out)
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
