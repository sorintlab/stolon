// Copyright 2026 PgVillage
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

// Package cmd is a package which provides utilities that underly the specific command
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	cluster "github.com/pgvillage-tools/stolon/api/v1"
	cmdcommon "github.com/pgvillage-tools/stolon/cmd"
	"github.com/pgvillage-tools/stolon/internal/logging"
	ststore "github.com/pgvillage-tools/stolon/internal/store"

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
	cmdReadClusterData.PersistentFlags().BoolVar(
		&readClusterdataOpts.pretty,
		"pretty",
		false,
		"pretty print",
	)
	cmdClusterData.AddCommand(cmdReadClusterData)

	cmdWriteClusterData.PersistentFlags().StringVarP(
		&writeClusterdataOpts.file,
		"file",
		"f",
		"",
		"file containing the new cluster data",
	)
	cmdWriteClusterData.PersistentFlags().BoolVarP(
		&writeClusterdataOpts.forceYes,
		"yes",
		"y",
		false,
		"don't ask for confirmation",
	)
	cmdClusterData.AddCommand(cmdWriteClusterData)

	CmdStolonCtl.AddCommand(cmdClusterData)
}

func readClusterdata(_ *cobra.Command, _ []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	e, err := cmdcommon.NewStore(ctx, &cfg.CommonConfig)
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

func isSafeToWriteClusterData(store ststore.Store) error {
	if cd, _, err := store.GetClusterData(context.TODO()); err != nil {
		return err
	} else if cd != nil {
		if !writeClusterdataOpts.forceYes {
			return errors.New("WARNING: cluster data already available use --yes to override")
		}
		stdout("WARNING: The current cluster data will be removed")
	}
	return nil
}

func clusterData(data []byte) (*cluster.Data, error) {
	cd := cluster.Data{}
	err := json.Unmarshal(data, &cd)
	return &cd, err
}

func writeClusterdata(reader io.Reader, s ststore.Store) error {
	data, err := io.ReadAll(reader)
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
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	var reader io.Reader
	if writeClusterdataOpts.file == "" || writeClusterdataOpts.file == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(writeClusterdataOpts.file)
		if err != nil {
			die("cannot read file: %v", err)
		}
		if err := file.Close(); err != nil {
			logger.Fatal().AnErr("err", err).Msg("closing file failed")
		}
		reader = file
	}
	s, err := cmdcommon.NewStore(ctx, &cfg.CommonConfig)
	if err != nil {
		die("failed to create new store %v", err)
	}
	if err := writeClusterdata(reader, s); err != nil {
		die("%v", err)
	}
}
