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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	cmdcommon "github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/store"

	"github.com/spf13/cobra"
)

var cmdClusterData = &cobra.Command{
	Use:   "clusterdata",
	Short: "Manage current cluster data",
}

type clusterdataReadOptions struct {
	pretty bool
}

var readClusterdataOpts clusterdataReadOptions

type clusterdataWriteOptions struct {
	file     string
	forceYes bool
}

var writeClusterdataOpts clusterdataWriteOptions

var cmdReadClusterData = &cobra.Command{
	Use:   "read",
	Run:   readClusterdata,
	Short: "Retrieve the current cluster data",
}

var cmdWriteClusterData = &cobra.Command{
	Use:   "write",
	Run:   runWriteClusterdata,
	Short: "Write cluster data",
}

func init() {
	cmdReadClusterData.PersistentFlags().BoolVar(&readClusterdataOpts.pretty, "pretty", false, "pretty print")
	cmdClusterData.AddCommand(cmdReadClusterData)

	cmdWriteClusterData.PersistentFlags().StringVarP(&writeClusterdataOpts.file, "file", "f", "", "file containing the new cluster data")
	cmdWriteClusterData.PersistentFlags().BoolVarP(&writeClusterdataOpts.forceYes, "yes", "y", false, "don't ask for confirmation")
	cmdClusterData.AddCommand(cmdWriteClusterData)

	CmdStolonCtl.AddCommand(cmdClusterData)
}

func readClusterdata(cmd *cobra.Command, args []string) {
	e, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		die("%v", err)
	}

	cd, _, err := getClusterData(e)
	if err != nil {
		die("%v", err)
	}
	if cd.Cluster == nil {
		die("no cluster clusterdata available")
	}
	var clusterdataj []byte
	if readClusterdataOpts.pretty {
		clusterdataj, err = json.MarshalIndent(cd, "", "\t")
		if err != nil {
			die("failed to marshall clusterdata: %v", err)
		}
	} else {
		clusterdataj, err = json.Marshal(cd)
		if err != nil {
			die("failed to marshall clusterdata: %v", err)
		}
	}
	stdout("%s", clusterdataj)
}

func isSafeToWriteClusterData(store store.Store) error {
	if cd, _, err := store.GetClusterData(context.TODO()); err != nil {
		return err
	} else if cd != nil {
		if !writeClusterdataOpts.forceYes {
			return fmt.Errorf("WARNING: cluster data already available use --yes to override")
		} else {
			stdout("WARNING: The current cluster data will be removed")
		}
	}
	return nil
}

func clusterData(data []byte) (*cluster.ClusterData, error) {
	cd := cluster.ClusterData{}
	err := json.Unmarshal(data, &cd)
	return &cd, err
}

func writeClusterdata(reader io.Reader, s store.Store) error {
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("error while reading data: %v", err)
	}

	cd, err := clusterData(data)

	if err != nil {
		return fmt.Errorf("invalid cluster data: %v", err)
	}

	if err = isSafeToWriteClusterData(s); err != nil {
		return err
	}

	err = s.PutClusterData(context.TODO(), cd)

	if err != nil {
		return fmt.Errorf("failed to write cluster data into new store %v", err)
	}
	stdout("successfully wrote cluster data into the new store")
	return nil
}

func runWriteClusterdata(_ *cobra.Command, _ []string) {
	var reader io.Reader
	if writeClusterdataOpts.file == "" || writeClusterdataOpts.file == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(writeClusterdataOpts.file)
		if err != nil {
			die("cannot read file: %v", err)
		}
		defer file.Close()
		reader = file
	}
	s, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		die("failed to create new store %v", err)
	}
	if err := writeClusterdata(reader, s); err != nil {
		die("%v", err)
	}
}
