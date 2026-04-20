// Copyright 2026 PgVillage
// Copyright 2019 Sorint.lab
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
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pgvillage-tools/stolon/cmd"
	"github.com/pgvillage-tools/stolon/cmd/stolonctl/cmd/register"
	"github.com/pgvillage-tools/stolon/internal/logging"
	stolonstore "github.com/pgvillage-tools/stolon/internal/store"
	"github.com/spf13/cobra"
)

// Register command to register stolon master and slave for service discovery
var Register = &cobra.Command{
	Use:     "register",
	Short:   "Register stolon keepers for service discovery",
	Run:     runRegister,
	Version: cmd.Version,
}

var rCfg register.Config

const sleepinterval = 10

func init() {
	Register.PersistentFlags().StringVar(
		&rCfg.Backend,
		"register-backend",
		"consul",
		"register backend type (consul)",
	)
	Register.PersistentFlags().StringVar(
		&rCfg.Endpoints,
		"register-endpoints",
		"http://127.0.0.1:8500",
		//revive:disable-next-line
		"a comma-delimited list of register endpoints (use https scheme for tls communication) defaults: http://127.0.0.1:8500 for consul")
	Register.PersistentFlags().StringVar(
		&rCfg.TLSCertFile,
		"register-cert-file",
		"",
		"certificate file for client identification to the register",
	)
	Register.PersistentFlags().StringVar(
		&rCfg.TLSKeyFile,
		"register-key",
		"",
		"private key file for client identification to the register",
	)
	Register.PersistentFlags().BoolVar(
		&rCfg.TLSInsecureSkipVerify,
		"register-skip-tls-verify",
		false,
		"skip register certificate verification (insecure!!!)",
	)
	Register.PersistentFlags().StringVar(
		&rCfg.TLSCAFile,
		"register-ca-file",
		"",
		"verify certificates of HTTPS-enabled register servers using this CA bundle",
	)
	Register.PersistentFlags().BoolVar(&rCfg.RegisterMaster,
		"register-master",
		false,
		"register master as well for service discovery (use it with caution!!!)",
	)
	Register.PersistentFlags().StringVar(
		&rCfg.TagMasterAs,
		"tag-master-as",
		"master",
		"a comma-delimited list of tag to be used when registering master",
	)
	Register.PersistentFlags().StringVar(
		&rCfg.TagSlaveAs,
		"tag-slave-as",
		"slave",
		"a comma-delimited list of tag to be used when registering slave",
	)
	Register.PersistentFlags().BoolVar(
		&cfg.Debug,
		"dbug",
		false,
		"enable debug logging",
	)
	Register.PersistentFlags().IntVar(
		&rCfg.SleepInterval,
		"sleep-interval",
		sleepinterval,
		"number of seconds to sleep before probing for change",
	)
	CmdStolonCtl.AddCommand(Register)
}

func sleepInterval() time.Duration {
	return time.Duration(rCfg.SleepInterval) * time.Second
}

func checkConfig(cfg *config, rCfg *register.Config) error {
	if err := cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		return err
	}
	return rCfg.Validate()
}

func runRegister(c *cobra.Command, _ []string) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	logging.SetStaticLevel(cfg.LogLevel)
	if cfg.Debug {
		logging.SetStaticLevel("debug")
	}
	if cmd.IsColorLoggerEnable(c, &cfg.CommonConfig) {
		logging.EnableColor()
	}

	if err := checkConfig(&cfg, &rCfg); err != nil {
		die("%s", err.Error())
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	if err := registerCluster(ctx, sigs, &cfg, &rCfg); err != nil {
		die("%s", err.Error())
	}
}

func registerCluster(ctx context.Context, sigs chan os.Signal, cfg *config, rCfg *register.Config) error {
	s, err := cmd.NewStore(ctx, &cfg.CommonConfig)
	if err != nil {
		return err
	}

	endCh := make(chan struct{})
	timerCh := time.NewTimer(0).C

	service, err := register.NewServiceDiscovery(rCfg)
	if err != nil {
		return err
	}

	for {
		select {
		case <-sigs:
			return nil
		case <-timerCh:
			go func() {
				checkAndRegisterMasterAndSlaves(ctx, cfg.ClusterName, s, service, rCfg.RegisterMaster)
				endCh <- struct{}{}
			}()
		case <-endCh:
			timerCh = time.NewTimer(sleepInterval()).C
		}
	}
}

func checkAndRegisterMasterAndSlaves(
	ctx context.Context,
	clusterName string,
	store stolonstore.Store,
	discovery register.ServiceDiscovery,
	registerMaster bool,
) {
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	discoveredServices, err := discovery.Services(clusterName)
	if err != nil {
		logger.Error().AnErr("err", err).Msg("unable to get info about existing services")
		return
	}

	existingServices, err := getExistingServices(ctx, clusterName, store, registerMaster)
	if err != nil {
		logger.Error().AnErr("err", err).Msg("skipping")
		return
	}
	logger.Debug().Any("services", existingServices).Msg("found services")

	diff := existingServices.Diff(discoveredServices)

	for _, removed := range diff.Removed {
		deRegisterService(ctx, discovery, &removed)
	}
	for _, added := range diff.Added {
		registerService(ctx, discovery, &added)
	}
}

func getExistingServices(
	ctx context.Context,
	clusterName string,
	store stolonstore.Store,
	includeMaster bool,
) (register.ServiceInfos, error) {
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	cluster, err := register.NewCluster(clusterName, rCfg, store)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster data: %v", err)
	}

	result := register.ServiceInfos{}
	infos, err := cluster.ServiceInfos()
	if err != nil {
		return nil, fmt.Errorf("cannot get service infos: %v", err)
	}

	for uid, info := range infos {
		if !includeMaster && info.IsMaster {
			logger.Info().Msg("skipping registering master")
			continue
		}
		result[uid] = info
	}
	return result, nil
}

func registerService(ctx context.Context, service register.ServiceDiscovery, serviceInfo *register.ServiceInfo) {
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	if serviceInfo == nil {
		return
	}
	if err := service.Register(serviceInfo); err != nil {
		logger.Error().
			Str("service", serviceInfo.Name).
			Str("id", serviceInfo.ID).
			Any("tags", serviceInfo.Tags).
			AnErr("err", err).
			Msg("unable to register %s with uid %s as %v, reason: %s")
	} else {
		logger.Info().
			Str("service", serviceInfo.Name).
			Str("id", serviceInfo.ID).
			Any("tags", serviceInfo.Tags).
			Msg("successfully registered")
	}
}

func deRegisterService(ctx context.Context, service register.ServiceDiscovery, serviceInfo *register.ServiceInfo) {
	_, logger := logging.GetLogComponent(ctx, logging.CmdComponent)
	if serviceInfo == nil {
		return
	}
	if err := service.DeRegister(serviceInfo); err != nil {
		logger.Error().
			Str("service", serviceInfo.Name).
			Str("id", serviceInfo.ID).
			Any("tags", serviceInfo.Tags).
			AnErr("err", err).
			Msg("")
	} else {
		logger.Info().
			Str("service", serviceInfo.Name).
			Str("id", serviceInfo.ID).
			Any("tags", serviceInfo.Tags).
			Msg("successfully deregistered service")
	}
}
