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
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/cmd/stolonctl/cmd/register"
	slog "github.com/sorintlab/stolon/internal/log"
	"github.com/sorintlab/stolon/internal/store"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

//Register command to register stolon master and slave for service discovery
var Register = &cobra.Command{
	Use:     "register",
	Short:   "Register stolon keepers for service discovery",
	Run:     runRegister,
	Version: cmd.Version,
}

var rCfg register.Config
var log = slog.S()

func init() {
	Register.PersistentFlags().StringVar(&rCfg.Backend, "register-backend", "consul", "register backend type (consul)")
	Register.PersistentFlags().StringVar(&rCfg.Endpoints, "register-endpoints", "http://127.0.0.1:8500", "a comma-delimited list of register endpoints (use https scheme for tls communication) defaults: http://127.0.0.1:8500 for consul")
	Register.PersistentFlags().StringVar(&rCfg.TLSCertFile, "register-cert-file", "", "certificate file for client identification to the register")
	Register.PersistentFlags().StringVar(&rCfg.TLSKeyFile, "register-key", "", "private key file for client identification to the register")
	Register.PersistentFlags().BoolVar(&rCfg.TLSInsecureSkipVerify, "register-skip-tls-verify", false, "skip register certificate verification (insecure!!!)")
	Register.PersistentFlags().StringVar(&rCfg.TLSCAFile, "register-ca-file", "", "verify certificates of HTTPS-enabled register servers using this CA bundle")
	Register.PersistentFlags().BoolVar(&rCfg.RegisterMaster, "register-master", false, "register master as well for service discovery (use it with caution!!!)")
	Register.PersistentFlags().StringVar(&rCfg.TagMasterAs, "tag-master-as", "master", "a comma-delimited list of tag to be used when registering master")
	Register.PersistentFlags().StringVar(&rCfg.TagSlaveAs, "tag-slave-as", "slave", "a comma-delimited list of tag to be used when registering slave")
	Register.PersistentFlags().BoolVar(&cfg.Debug, "debug", false, "enable debug logging")
	Register.PersistentFlags().IntVar(&rCfg.SleepInterval, "sleep-interval", 10, "number of seconds to sleep before probing for change")
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
	switch cfg.LogLevel {
	case "error":
		slog.SetLevel(zap.ErrorLevel)
	case "warn":
		slog.SetLevel(zap.WarnLevel)
	case "info":
		slog.SetLevel(zap.InfoLevel)
	case "debug":
		slog.SetLevel(zap.DebugLevel)
	default:
		die("invalid log level: %v", cfg.LogLevel)
	}
	if cfg.Debug {
		slog.SetDebug()
	}
	if cmd.IsColorLoggerEnable(c, &cfg.CommonConfig) {
		log = slog.SColor()
	}

	if err := checkConfig(&cfg, &rCfg); err != nil {
		die(err.Error())
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	if err := registerCluster(sigs, &cfg, &rCfg); err != nil {
		die(err.Error())
	}
}

func registerCluster(sigs chan os.Signal, cfg *config, rCfg *register.Config) error {
	s, err := cmd.NewStore(&cfg.CommonConfig)
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
				checkAndRegisterMasterAndSlaves(cfg.ClusterName, s, service, rCfg.RegisterMaster)
				endCh <- struct{}{}
			}()
		case <-endCh:
			timerCh = time.NewTimer(sleepInterval()).C
		}
	}
}

func checkAndRegisterMasterAndSlaves(clusterName string, store store.Store, discovery register.ServiceDiscovery, registerMaster bool) {
	discoveredServices, err := discovery.Services(clusterName)
	if err != nil {
		log.Errorf("unable to get info about existing services: %v", err)
		return
	}

	existingServices, err := getExistingServices(clusterName, store, registerMaster)
	if err == nil {
		log.Debugf("found services %v", existingServices)
	} else {
		log.Errorf("%s skipping", err.Error())
		return
	}

	diff := existingServices.Diff(discoveredServices)

	for _, removed := range diff.Removed {
		deRegisterService(discovery, &removed)
	}
	for _, added := range diff.Added {
		registerService(discovery, &added)
	}
}

func getExistingServices(clusterName string, store store.Store, includeMaster bool) (register.ServiceInfos, error) {
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
			log.Infof("skipping registering master")
			continue
		}
		result[uid] = info
	}
	return result, nil
}

func registerService(service register.ServiceDiscovery, serviceInfo *register.ServiceInfo) {
	if serviceInfo == nil {
		return
	}
	if err := service.Register(serviceInfo); err != nil {
		log.Errorf("unable to register %s with uid %s as %v, reason: %s", serviceInfo.Name, serviceInfo.ID, serviceInfo.Tags, err.Error())
	} else {
		log.Infof("successfully registered %s with uid %s as %v", serviceInfo.Name, serviceInfo.ID, serviceInfo.Tags)
	}
}

func deRegisterService(service register.ServiceDiscovery, serviceInfo *register.ServiceInfo) {
	if serviceInfo == nil {
		return
	}
	if err := service.DeRegister(serviceInfo); err != nil {
		log.Errorf("unable to deregister %s with uid %s as %v, reason: %s", serviceInfo.Name, serviceInfo.ID, serviceInfo.Tags, err.Error())
	} else {
		log.Infof("successfully deregistered %s with uid %s as %v", serviceInfo.Name, serviceInfo.ID, serviceInfo.Tags)
	}
}
