// Copyright 2026 PgVillage
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
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	cluster "github.com/pgvillage-tools/stolon/api/v1"
	"github.com/pgvillage-tools/stolon/cmd"
	"github.com/pgvillage-tools/stolon/internal/flagutil"
	"github.com/pgvillage-tools/stolon/internal/logging"
	"github.com/pgvillage-tools/stolon/internal/store"

	"github.com/spf13/cobra"
)

const (
	maxRetries = 3
)

// CmdStolonCtl defines a cobra command to execute when running stolonctl
var CmdStolonCtl = &cobra.Command{
	Use:     "stolonctl",
	Short:   "stolon command line client",
	Version: cmd.Version,
	PersistentPreRun: func(c *cobra.Command, _ []string) {
		if c.Name() != "stolonctl" && c.Name() != "version" {
			if err := cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
				die("%s", err.Error())
			}
		}
	},
	// just defined to make --version work
	Run: func(c *cobra.Command, _ []string) { _ = c.Help() },
}

type config struct {
	cmd.CommonConfig
}

var cfg config

func init() {
	cfg.IsStolonCtl = true
	cmd.AddCommonFlags(CmdStolonCtl, &cfg.CommonConfig)
}

var cmdVersion = &cobra.Command{
	Use: "version",
	Run: func(_ *cobra.Command, _ []string) {
		stdout("stolonctl version %s", cmd.Version)
	},
	Short: "Display the version",
}

func init() {
	CmdStolonCtl.AddCommand(cmdVersion)
}

// Execute is run when stolonctl is executed
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.CmdComponent)
	if err := flagutil.SetFlagsFromEnv(CmdStolonCtl.PersistentFlags(), "STOLONCTL"); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}
	if err := CmdStolonCtl.Execute(); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}
}

func stderr(format string, a ...any) {
	out := fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stderr, strings.TrimSuffix(out, "\n"))
}

func stdout(format string, a ...any) {
	out := fmt.Sprintf(format, a...)
	if _, err := fmt.Fprintln(os.Stdout, strings.TrimSuffix(out, "\n")); err != nil {
		log.Fatalf("failed to write to stdout: %v", err)
	}
}

func die(format string, a ...any) {
	stderr(format, a...)
	os.Exit(1)
}

func getClusterData(e store.Store) (*cluster.Data, *store.KVPair, error) {
	cd, pair, err := e.GetClusterData(context.TODO())
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get cluster data: %v", err)
	}
	if cd == nil {
		return nil, nil, fmt.Errorf("nil cluster data: %v", err)
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		return nil, nil, fmt.Errorf("unsupported cluster data format version %d", cd.FormatVersion)
	}
	if err := cd.Cluster.Spec.Validate(); err != nil {
		return nil, nil, fmt.Errorf("clusterdata validation failed: %v", err)
	}
	return cd, pair, nil
}

func askConfirmation(message string) (bool, error) {
	in := bufio.NewReader(os.Stdin)
	for {
		if _, err := fmt.Fprint(os.Stdout, message); err != nil {
			log.Fatalf("failed to print to stdout: %v", err)
		}
		input, err := in.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("error reading input: %v", err)
		}
		switch input {
		case "yes\n":
			return true, nil
		case "no\n":
			return false, nil
		default:
			stdout("Please enter 'yes' or 'no'")
		}
	}
}
