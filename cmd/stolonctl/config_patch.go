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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	etcdm "github.com/sorintlab/stolon/pkg/etcd"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/spf13/cobra"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/k8s.io/kubernetes/pkg/util/strategicpatch"
)

var cmdConfigPatch = &cobra.Command{
	Use:   "patch",
	Run:   configPatch,
	Short: "patch configuration",
}

type configPatchOpts struct {
	file string
}

var cpOpts configPatchOpts

func init() {
	cmdConfigPatch.PersistentFlags().StringVarP(&cpOpts.file, "file", "f", "", "file contaning the patch to apply to the current configuration")

	cmdConfig.AddCommand(cmdConfigPatch)
}

func patchConfig(e *etcdm.EtcdManager, ncj []byte) error {
	curnc, err := getConfig(e)
	if err != nil {
		return fmt.Errorf("cannot get config: %v", err)
	}
	curncj, err := json.Marshal(curnc)
	if err != nil {
		return fmt.Errorf("failed to marshall config: %v", err)
	}

	pcj, err := strategicpatch.StrategicMergePatch(curncj, ncj, &cluster.Config{})
	if err != nil {
		return fmt.Errorf("failed to merge patch config: %v", err)
	}

	if err = replaceConfig(e, pcj); err != nil {
		return err
	}

	return nil
}

func configPatch(cmd *cobra.Command, args []string) {
	if len(args) > 1 {
		die("too many arguments")
	}
	if cpOpts.file == "" && len(args) < 1 {
		die("no patch provided as argument and no patch file provided (--file/-f option)")
	}
	if cpOpts.file != "" && len(args) == 1 {
		die("only one of patch provided as argument or patch file must provided (--file/-f option)")
	}

	config := []byte{}
	if len(args) == 1 {
		config = []byte(args[0])
	} else {
		var err error
		if cpOpts.file == "-" {
			config, err = ioutil.ReadAll(os.Stdin)
			if err != nil {
				die("cannot read config file from stdin: %v", err)
			}
		} else {
			config, err = ioutil.ReadFile(cpOpts.file)
			if err != nil {
				die("cannot read provided config file: %v", err)
			}
		}
	}

	etcdPath := filepath.Join(common.EtcdBasePath, cfg.clusterName)
	e, err := etcdm.NewEtcdManager(cfg.etcdEndpoints, etcdPath, common.DefaultEtcdRequestTimeout)
	if err != nil {
		die("error: %v", err)
	}

	if err = patchConfig(e, config); err != nil {
		die("failed to patch config: %v", err)
	}
}
