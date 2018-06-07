// Copyright 2016 Sorint.lab
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
	"io/ioutil"
	"os"

	cmdcommon "github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/store"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

var cmdUpdate = &cobra.Command{
	Use:   "update",
	Run:   update,
	Short: "Update a cluster specification",
}

type updateOptions struct {
	patch bool
	file  string
}

var updateOpts updateOptions

func init() {
	cmdUpdate.PersistentFlags().BoolVarP(&updateOpts.patch, "patch", "p", false, "patch the current cluster specification instead of replacing it")
	cmdUpdate.PersistentFlags().StringVarP(&updateOpts.file, "file", "f", "", "file containing a complete cluster specification or a patch to apply to the current cluster specification")

	CmdStolonCtl.AddCommand(cmdUpdate)
}

func patchClusterSpec(cs *cluster.ClusterSpec, p []byte) (*cluster.ClusterSpec, error) {
	csj, err := json.Marshal(cs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster spec: %v", err)
	}

	newcsj, err := strategicpatch.StrategicMergePatch(csj, p, &cluster.ClusterSpec{})
	if err != nil {
		return nil, fmt.Errorf("failed to merge patch cluster spec: %v", err)
	}
	var newcs *cluster.ClusterSpec
	if err := json.Unmarshal(newcsj, &newcs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patched cluster spec: %v", err)
	}
	return newcs, nil
}

func update(cmd *cobra.Command, args []string) {
	if len(args) > 1 {
		die("too many arguments")
	}
	if updateOpts.file == "" && len(args) < 1 {
		die("no cluster spec provided as argument and no file provided (--file/-f option)")
	}
	if updateOpts.file != "" && len(args) == 1 {
		die("only one of cluster spec provided as argument or file must provided (--file/-f option)")
	}

	data := []byte{}
	if len(args) == 1 {
		data = []byte(args[0])
	} else {
		var err error
		if updateOpts.file == "-" {
			data, err = ioutil.ReadAll(os.Stdin)
			if err != nil {
				die("cannot read from stdin: %v", err)
			}
		} else {
			data, err = ioutil.ReadFile(updateOpts.file)
			if err != nil {
				die("cannot read file: %v", err)
			}
		}
	}

	e, err := cmdcommon.NewStore(&cfg.CommonConfig)
	if err != nil {
		die("%v", err)
	}

	retry := 0
	for retry < maxRetries {
		cd, pair, err := getClusterData(e)
		if err != nil {
			die("%v", err)
		}
		if cd.Cluster == nil {
			die("no cluster spec available")
		}
		if cd.Cluster.Spec == nil {
			die("no cluster spec available")
		}

		var newcs *cluster.ClusterSpec
		if updateOpts.patch {
			newcs, err = patchClusterSpec(cd.Cluster.Spec, data)
			if err != nil {
				die("failed to patch cluster spec: %v", err)
			}
		} else {
			if err = json.Unmarshal(data, &newcs); err != nil {
				die("failed to unmarshal cluster spec: %v", err)
			}
		}
		if err = cd.Cluster.UpdateSpec(newcs); err != nil {
			die("Cannot update cluster spec: %v", err)
		}

		// retry if cd has been modified between reading and writing
		_, err = e.AtomicPutClusterData(context.TODO(), cd, pair)
		if err != nil {
			if err == store.ErrKeyModified {
				retry++
				continue
			}
			die("cannot update cluster data: %v", err)
		}
		break
	}
	if retry == maxRetries {
		die("failed to update cluster data after %d retries", maxRetries)
	}
}
