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
	"path/filepath"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	etcdm "github.com/sorintlab/stolon/pkg/etcd"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/spf13/cobra"
)

var cmdConfigGet = &cobra.Command{
	Use:   "get",
	Run:   configGet,
	Short: "get configuration",
}

func init() {
	cmdConfig.AddCommand(cmdConfigGet)
}

func getConfig(e *etcdm.EtcdManager) (*cluster.NilConfig, error) {
	cv, _, err := e.GetClusterView()
	if err != nil {
		return nil, fmt.Errorf("cannot get clusterview: %v", err)
	}
	if cv == nil {
		return nil, fmt.Errorf("no clusterview available")
	}
	cfg := cv.Config
	if cfg == nil {
		return nil, nil
	}
	return cfg, nil
}

func configGet(cmd *cobra.Command, args []string) {
	etcdPath := filepath.Join(common.EtcdBasePath, cfg.clusterName)
	e, err := etcdm.NewEtcdManager(cfg.etcdEndpoints, etcdPath, common.DefaultEtcdRequestTimeout)
	if err != nil {
		die("error: %v", err)
	}

	cfg, err := getConfig(e)
	if err != nil {
		die("error: %v", err)
	}

	if cfg == nil {
		stdout("config is not defined")
		os.Exit(0)
	}

	cfgj, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		die("failed to marshall config: %v", err)
	}

	stdout(string(cfgj))
}
