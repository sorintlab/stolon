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

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	cmdcommon "github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/store"

	"github.com/spf13/cobra"
)

var cmdStatus = &cobra.Command{
	Use:   "status",
	Run:   status,
	Short: "Display the current cluster status",
}

type StatusOptions struct {
	Format string
}

var statusOpts StatusOptions

func init() {
	cmdStatus.PersistentFlags().StringVarP(&statusOpts.Format, "format", "f", "", "output format")
	CmdStolonCtl.AddCommand(cmdStatus)
}

type Status struct {
	Sentinels []SentinelStatus `json:"sentinels"`
	Proxies   []ProxyStatus    `json:"proxies"`
	Keepers   []KeeperStatus   `json:"keepers"`
	Cluster   ClusterStatus    `json:"cluster"`
}

type SentinelStatus struct {
	UID    string `json:"uid"`
	Leader bool   `json:"leader"`
}

type ProxyStatus struct {
	UID        string `json:"uid"`
	Generation int64  `json:"generation"`
}

type KeeperStatus struct {
	UID                 string `json:"uid"`
	ListenAddress       string `json:"listen_address"`
	Healthy             bool   `json:"healthy"`
	PgHealthy           bool   `json:"pg_healthy"`
	PgWantedGeneration  int64  `json:"pg_wanted_generation"`
	PgCurrentGeneration int64  `json:"pg_current_generation"`
}

type ClusterStatus struct {
	Available       bool   `json:"available"`
	MasterKeeperUID string `json:"master_keeper_uid"`
	MasterDBUID     string `json:"master_db_uid"`
}

func status(cmd *cobra.Command, args []string) {
	status, generateErr := generateStatus()
	switch statusOpts.Format {
	case "json":
		renderJSON(status, generateErr)
	case "text":
		renderText(status, generateErr)
	case "":
		renderText(status, generateErr)
	default:
		die("unrecognised output format %s", statusOpts.Format)
	}
}

func renderJSON(status Status, generateErr error) {
	if generateErr != nil {
		marshalJSON(generateErr)
	} else {
		marshalJSON(status)
	}
}

func marshalJSON(value interface{}) {
	output, err := json.MarshalIndent(value, "", "\t")
	if err != nil {
		die("failed to marshal error: %v", err)
	}
	stdout("%s", output)
}

func renderText(status Status, generateErr error) {
	if generateErr != nil {
		die("%v", generateErr)
	}

	tabOut := new(tabwriter.Writer)
	tabOut.Init(os.Stdout, 0, 8, 1, '\t', 0)

	stdout("=== Active sentinels ===")
	stdout("")
	if len(status.Sentinels) == 0 {
		stdout("No active sentinels")
	} else {
		fmt.Fprintf(tabOut, "ID\tLEADER\n")
		for _, s := range status.Sentinels {
			fmt.Fprintf(tabOut, "%s\t%t\n", s.UID, s.Leader)
			tabOut.Flush()
		}
	}

	stdout("")
	stdout("=== Active proxies ===")
	stdout("")
	if len(status.Proxies) == 0 {
		stdout("No active proxies")
	} else {
		fmt.Fprintf(tabOut, "ID\n")
		for _, p := range status.Proxies {
			fmt.Fprintf(tabOut, "%s\n", p.UID)
			tabOut.Flush()
		}
	}

	stdout("")
	stdout("=== Keepers ===")
	stdout("")
	if len(status.Keepers) == 0 {
		stdout("No keepers available")
		stdout("")
	} else {
		fmt.Fprintf(tabOut, "UID\tHEALTHY\tPG LISTENADDRESS\tPG HEALTHY\tPG WANTEDGENERATION\tPG CURRENTGENERATION\n")
		for _, k := range status.Keepers {
			fmt.Fprintf(tabOut, "%s\t%t\t%s\t%t\t%d\t%d\t\n", k.UID, k.Healthy, k.ListenAddress, k.PgHealthy, k.PgWantedGeneration, k.PgCurrentGeneration)
			tabOut.Flush()
		}
	}

	if status.Cluster.MasterKeeperUID == "" {
		stdout("No cluster available")
	} else {
		stdout("")
		stdout("=== Cluster Info ===")
		stdout("")
		if status.Cluster.MasterKeeperUID != "" {
			stdout("Master Keeper: %s", status.Cluster.MasterKeeperUID)
		} else {
			stdout("Master Keeper: (none)")
		}
	}

	// This tree data isn't currently available in the Status struct
	e, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		die("%v", err)
	}
	cd, _, err := getClusterData(e)
	if err != nil {
		die("%v", err)
	}
	if status.Cluster.MasterDBUID != "" {
		stdout("")
		stdout("===== Keepers/DB tree =====")
		stdout("")
		printTree(status.Cluster.MasterDBUID, cd, 0, "", true)
	}
	stdout("")
}

func printTree(dbuid string, cd *cluster.ClusterData, level int, prefix string, tail bool) {
	// skip not existing db: specified as a follower but not available in the
	// cluster spec (this should happen only when doing a stolonctl
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

func generateStatus() (Status, error) {
	status := Status{}
	tabOut := new(tabwriter.Writer)
	tabOut.Init(os.Stdout, 0, 8, 1, '\t', 0)

	e, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		return status, err
	}

	election, err := cmdcommon.NewElection(&cfg.CommonConfig, "")
	if err != nil {
		return status, err
	}

	lsid, err := election.Leader()
	if err != nil && err != store.ErrElectionNoLeader {
		return status, err
	}

	sentinelsInfo, err := e.GetSentinelsInfo(context.TODO())
	if err != nil {
		return status, err
	}

	sentinels := make([]SentinelStatus, 0)
	sort.Sort(sentinelsInfo)
	for _, si := range sentinelsInfo {
		leader := lsid != "" && si.UID == lsid
		sentinels = append(sentinels, SentinelStatus{UID: si.UID, Leader: leader})
	}
	status.Sentinels = sentinels

	proxiesInfo, err := e.GetProxiesInfo(context.TODO())
	if err != nil {
		return status, err
	}
	proxiesInfoSlice := proxiesInfo.ToSlice()

	proxies := make([]ProxyStatus, 0)
	sort.Sort(proxiesInfoSlice)
	for _, pi := range proxiesInfoSlice {
		proxies = append(proxies, ProxyStatus{UID: pi.UID, Generation: pi.Generation})
	}
	status.Proxies = proxies

	cd, _, err := getClusterData(e)
	if err != nil {
		return status, err
	}

	keepers := make([]KeeperStatus, 0)
	kssKeys := cd.Keepers.SortedKeys()
	for _, kuid := range kssKeys {
		k := cd.Keepers[kuid]
		db := cd.FindDB(k)
		dbListenAddress := "(no db assigned)"
		var (
			pgHealthy           bool
			pgCurrentGeneration int64
			pgWantedGeneration  int64
		)
		if db != nil {
			pgHealthy = db.Status.Healthy
			pgCurrentGeneration = db.Status.CurrentGeneration
			pgWantedGeneration = db.Generation

			dbListenAddress = "(unknown)"
			if db.Status.ListenAddress != "" {
				dbListenAddress = fmt.Sprintf("%s:%s", db.Status.ListenAddress, db.Status.Port)
			}
		}
		keeper := KeeperStatus{
			UID:                 kuid,
			ListenAddress:       dbListenAddress,
			Healthy:             k.Status.Healthy,
			PgHealthy:           pgHealthy,
			PgWantedGeneration:  pgWantedGeneration,
			PgCurrentGeneration: pgCurrentGeneration,
		}
		keepers = append(keepers, keeper)
	}
	status.Keepers = keepers

	cluster := ClusterStatus{}
	if cd.Cluster == nil || cd.DBs == nil {
		cluster.Available = false
	} else {
		master := cd.Cluster.Status.Master
		cluster.Available = true

		if master != "" {
			cluster.MasterDBUID = cd.DBs[master].UID
			cluster.MasterKeeperUID = cd.Keepers[cd.DBs[master].Spec.KeeperUID].UID
		}
	}
	status.Cluster = cluster

	return status, nil
}
