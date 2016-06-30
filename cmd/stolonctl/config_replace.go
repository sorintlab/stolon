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
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/store"

	"github.com/spf13/cobra"
)

var cmdConfigReplace = &cobra.Command{
	Use:   "replace",
	Run:   configReplace,
	Short: "replace configuration",
}

type configReplaceOpts struct {
	file string
}

var crOpts configReplaceOpts

func init() {
	cmdConfigReplace.PersistentFlags().StringVarP(&crOpts.file, "file", "f", "", "file contaning the configuration to replace")

	cmdConfig.AddCommand(cmdConfigReplace)
}

func replaceConfig(e *store.StoreManager, config []byte) error {
	lsid, err := e.GetLeaderSentinelId()
	if err != nil {
		die("cannot get leader sentinel info")
	}

	lsi, _, err := e.GetSentinelInfo(lsid)
	if lsi == nil {
		return fmt.Errorf("leader sentinel info not available")
	}

	req, err := http.NewRequest("PUT", fmt.Sprintf("http://%s:%s/config/current", lsi.ListenAddress, lsi.Port), bytes.NewReader(config))
	if err != nil {
		return fmt.Errorf("cannot create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error setting config: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("error setting config: leader sentinel returned non ok code: %s", res.Status)
	}
	return nil
}

func configReplace(cmd *cobra.Command, args []string) {
	if crOpts.file == "" {
		die("no config file provided (--file/-f option)")
	}

	config := []byte{}
	var err error
	if crOpts.file == "-" {
		config, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			die("cannot read config file from stdin: %v", err)
		}
	} else {
		config, err = ioutil.ReadFile(crOpts.file)
		if err != nil {
			die("cannot read provided config file: %v", err)
		}
	}

	storePath := filepath.Join(common.StoreBasePath, cfg.clusterName)

	kvstore, err := store.NewStore(store.Backend(cfg.storeBackend), cfg.storeEndpoints)
	if err != nil {
		die("cannot create store: %v", err)
	}
	e := store.NewStoreManager(kvstore, storePath)

	if err = replaceConfig(e, config); err != nil {
		die("error: %v", err)
	}
}
