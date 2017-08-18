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
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/flagutil"
	slog "github.com/sorintlab/stolon/pkg/log"
	"github.com/sorintlab/stolon/pkg/postgresql"
	pg "github.com/sorintlab/stolon/pkg/postgresql"
	"github.com/sorintlab/stolon/pkg/store"
	"github.com/sorintlab/stolon/pkg/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

var log = slog.S()

var cmdKeeper = &cobra.Command{
	Use: "stolon-keeper",
	Run: keeper,
}

type KeeperLocalState struct {
	UID        string
	ClusterUID string
}

type DBLocalState struct {
	UID        string
	Generation int64
	// Initializing registers when the db is initializing. Needed to detect
	// when the initialization has failed.
	Initializing bool
	// InitPGParameters contains the postgres parameter after the
	// initialization
	InitPGParameters common.Parameters
}

type config struct {
	uid                     string
	storeBackend            string
	storeEndpoints          string
	storeCertFile           string
	storeKeyFile            string
	storeCAFile             string
	storeSkipTlsVerify      bool
	dataDir                 string
	clusterName             string
	logLevel                string
	debug                   bool
	pgListenAddress         string
	pgPort                  string
	pgBinPath               string
	pgReplUsername          string
	pgReplPassword          string
	pgReplPasswordFile      string
	pgSUUsername            string
	pgSUPassword            string
	pgSUPasswordFile        string
	pgInitialSUUsername     string
	pgInitialSUPasswordFile string
}

var cfg config

func init() {
	user, err := util.GetUser()
	if err != nil {
		die("cannot get current user: %v", err)
	}

	cmdKeeper.PersistentFlags().StringVar(&cfg.uid, "id", "", "keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.uid, "uid", "", "keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.storeBackend, "store-backend", "", "store backend type (etcd or consul)")
	cmdKeeper.PersistentFlags().StringVar(&cfg.storeEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (use https scheme for tls communication) (defaults: http://127.0.0.1:2379 for etcd, http://127.0.0.1:8500 for consul)")
	cmdKeeper.PersistentFlags().StringVar(&cfg.storeCertFile, "store-cert-file", "", "certificate file for client identification to the store")
	cmdKeeper.PersistentFlags().StringVar(&cfg.storeKeyFile, "store-key", "", "private key file for client identification to the store")
	cmdKeeper.PersistentFlags().StringVar(&cfg.storeCAFile, "store-ca-file", "", "verify certificates of HTTPS-enabled store servers using this CA bundle")
	cmdKeeper.PersistentFlags().BoolVar(&cfg.storeSkipTlsVerify, "store-skip-tls-verify", false, "skip store certificate verification (insecure!!!)")
	cmdKeeper.PersistentFlags().StringVar(&cfg.dataDir, "data-dir", "", "data directory")
	cmdKeeper.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgListenAddress, "pg-listen-address", "localhost", "postgresql instance listening address")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgPort, "pg-port", "5432", "postgresql instance listening port")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgBinPath, "pg-bin-path", "", "absolute path to postgresql binaries. If empty they will be searched in the current PATH")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgReplUsername, "pg-repl-username", "", "postgres replication user name. Required. It'll be created on db initialization. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgReplPassword, "pg-repl-password", "", "postgres replication user password. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgReplPasswordFile, "pg-repl-passwordfile", "", "postgres replication user password file. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgSUUsername, "pg-su-username", user, "postgres superuser user name. Used for keeper managed instance access and pg_rewind based synchronization. It'll be created on db initialization. Defaults to the name of the effective user running stolon-keeper. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgSUPassword, "pg-su-password", "", "postgres superuser password. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgSUPasswordFile, "pg-su-passwordfile", "", "postgres superuser password file. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers)")
	cmdKeeper.PersistentFlags().StringVar(&cfg.logLevel, "log-level", "info", "debug, info (default), warn or error")
	cmdKeeper.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")

	cmdKeeper.PersistentFlags().MarkDeprecated("id", "please use --uid")
	cmdKeeper.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead")
}

var managedPGParameters = []string{
	"unix_socket_directories",
	"wal_level",
	"wal_keep_segments",
	"hot_standby",
	"listen_addresses",
	"port",
	"max_replication_slots",
	"max_wal_senders",
	"wal_log_hints",
	"synchronous_standby_names",
}

func stderr(format string, a ...interface{}) {
	out := fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stderr, strings.TrimSuffix(out, "\n"))
}

func stdout(format string, a ...interface{}) {
	out := fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stdout, strings.TrimSuffix(out, "\n"))
}

func die(format string, a ...interface{}) {
	stderr(format, a...)
	os.Exit(1)
}

func readPasswordFromFile(filepath string) (string, error) {
	fi, err := os.Lstat(filepath)
	if err != nil {
		return "", fmt.Errorf("unable to read password from file %s: %v", filepath, err)
	}

	if fi.Mode() > 0600 {
		//TODO: enforce this by exiting with an error. Kubernetes makes this file too open today.
		log.Warnw("password file permissions are too open. This file should only be readable to the user executing stolon! Continuing...", "file", filepath, "mode", fmt.Sprintf("%#o", fi.Mode()))
	}

	pwBytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return "", fmt.Errorf("unable to read password from file %s: %v", filepath, err)
	}
	return strings.TrimSpace(string(pwBytes)), nil
}

func (p *PostgresKeeper) mandatoryPGParameters() common.Parameters {
	return common.Parameters{
		"unix_socket_directories": common.PgUnixSocketDirectories,
		"wal_level":               "hot_standby",
		"wal_keep_segments":       "8",
		"hot_standby":             "on",
	}
}

func (p *PostgresKeeper) getSUConnParams(db, followedDB *cluster.DB) pg.ConnParams {
	cp := pg.ConnParams{
		"user":             p.pgSUUsername,
		"password":         p.pgSUPassword,
		"host":             followedDB.Status.ListenAddress,
		"port":             followedDB.Status.Port,
		"application_name": common.StolonName(db.UID),
		"dbname":           "postgres",
		"sslmode":          "disable",
	}
	return cp
}

func (p *PostgresKeeper) getReplConnParams(db, followedDB *cluster.DB) pg.ConnParams {
	return pg.ConnParams{
		"user":             p.pgReplUsername,
		"password":         p.pgReplPassword,
		"host":             followedDB.Status.ListenAddress,
		"port":             followedDB.Status.Port,
		"application_name": common.StolonName(db.UID),
		"sslmode":          "disable",
	}
}

func (p *PostgresKeeper) getLocalConnParams() pg.ConnParams {
	return pg.ConnParams{
		"user":     p.pgSUUsername,
		"password": p.pgSUPassword,
		"host":     common.PgUnixSocketDirectories,
		"port":     p.pgPort,
		"dbname":   "postgres",
		"sslmode":  "disable",
	}
}

func (p *PostgresKeeper) getLocalReplConnParams() pg.ConnParams {
	return pg.ConnParams{
		"user":     p.pgReplUsername,
		"password": p.pgReplPassword,
		"host":     common.PgUnixSocketDirectories,
		"port":     p.pgPort,
		"sslmode":  "disable",
	}
}

func (p *PostgresKeeper) createPGParameters(db *cluster.DB) common.Parameters {
	parameters := common.Parameters{}

	// Include init parameters if include config is required
	if db.Spec.IncludeConfig {
		for k, v := range p.dbLocalState.InitPGParameters {
			parameters[k] = v
		}
	}

	// Copy user defined pg parameters
	for k, v := range db.Spec.PGParameters {
		parameters[k] = v
	}

	// Add/Replace mandatory PGParameters
	for k, v := range p.mandatoryPGParameters() {
		parameters[k] = v
	}

	parameters["listen_addresses"] = fmt.Sprintf("127.0.0.1,%s", p.pgListenAddress)

	parameters["port"] = p.pgPort
	// TODO(sgotti) max_replication_slots needs to be at least the
	// number of existing replication slots or startup will
	// fail.
	// TODO(sgotti) changing max_replication_slots requires an
	// instance restart.
	parameters["max_replication_slots"] = strconv.FormatUint(uint64(db.Spec.MaxStandbys), 10)
	// Add some more wal senders, since also the keeper will use them
	// TODO(sgotti) changing max_wal_senders requires an instance restart.
	parameters["max_wal_senders"] = strconv.FormatUint(uint64(db.Spec.MaxStandbys+2+db.Spec.AdditionalWalSenders), 10)

	// required by pg_rewind (if data checksum is enabled it's ignored)
	if db.Spec.UsePgrewind {
		parameters["wal_log_hints"] = "on"
	}

	// Setup synchronous replication
	if db.Spec.SynchronousReplication && len(db.Spec.SynchronousStandbys) > 0 {
		synchronousStandbys := []string{}
		for _, synchronousStandby := range db.Spec.SynchronousStandbys {
			synchronousStandbys = append(synchronousStandbys, common.StolonName(synchronousStandby))
		}
		// TODO(sgotti) Find a way to detect the pg major version also
		// when the instance is empty. Parsing the postgres --version
		// is not always reliable (for example we can get `10devel` for
		// development version).
		// Now this will lead to problems starting the instance (or
		// warning reloading the parameters) with postgresql < 9.6
		if len(synchronousStandbys) > 1 {
			parameters["synchronous_standby_names"] = fmt.Sprintf("%d (%s)", len(synchronousStandbys), strings.Join(synchronousStandbys, ","))
		} else {
			parameters["synchronous_standby_names"] = strings.Join(synchronousStandbys, ",")
		}
	} else {
		parameters["synchronous_standby_names"] = ""
	}

	return parameters
}

func (p *PostgresKeeper) createRecoveryParameters(standbySettings *cluster.StandbySettings, archiveRecoverySettings *cluster.ArchiveRecoverySettings, recoveryTargetSettings *cluster.RecoveryTargetSettings) common.Parameters {
	parameters := common.Parameters{}

	if standbySettings != nil {
		parameters["standby_mode"] = "on"
		if standbySettings.PrimaryConninfo != "" {
			parameters["primary_conninfo"] = standbySettings.PrimaryConninfo
		}
		if standbySettings.PrimarySlotName != "" {
			parameters["primary_slot_name"] = standbySettings.PrimarySlotName
		}
		if standbySettings.RecoveryMinApplyDelay != "" {
			parameters["recovery_min_apply_delay"] = standbySettings.RecoveryMinApplyDelay
		}
	}

	if archiveRecoverySettings != nil {
		parameters["restore_command"] = archiveRecoverySettings.RestoreCommand
	}

	if recoveryTargetSettings == nil {
		parameters["recovery_target_timeline"] = "latest"
	} else {
		if recoveryTargetSettings.RecoveryTarget != "" {
			parameters["recovery_target"] = recoveryTargetSettings.RecoveryTarget
		}
		if recoveryTargetSettings.RecoveryTargetLsn != "" {
			parameters["recovery_target_lsn"] = recoveryTargetSettings.RecoveryTargetLsn
		}
		if recoveryTargetSettings.RecoveryTargetName != "" {
			parameters["recovery_target_name"] = recoveryTargetSettings.RecoveryTargetName
		}
		if recoveryTargetSettings.RecoveryTargetTime != "" {
			parameters["recovery_target_time"] = recoveryTargetSettings.RecoveryTargetTime
		}
		if recoveryTargetSettings.RecoveryTargetXid != "" {
			parameters["recovery_target_xid"] = recoveryTargetSettings.RecoveryTargetXid
		}

		if recoveryTargetSettings.RecoveryTargetTimeline != "" {
			parameters["recovery_target_timeline"] = recoveryTargetSettings.RecoveryTargetTimeline
		}
	}

	return parameters
}

type PostgresKeeper struct {
	cfg *config

	bootUUID string

	dataDir             string
	storeBackend        string
	storeEndpoints      string
	listenAddress       string
	port                string
	pgListenAddress     string
	pgPort              string
	pgBinPath           string
	pgReplUsername      string
	pgReplPassword      string
	pgSUUsername        string
	pgSUPassword        string
	pgInitialSUUsername string

	sleepInterval  time.Duration
	requestTimeout time.Duration

	e    *store.StoreManager
	pgm  *postgresql.Manager
	stop chan bool
	end  chan error

	localStateMutex  sync.Mutex
	keeperLocalState *KeeperLocalState
	dbLocalState     *DBLocalState

	pgStateMutex    sync.Mutex
	getPGStateMutex sync.Mutex
	lastPGState     *cluster.PostgresState
}

func NewPostgresKeeper(cfg *config, stop chan bool, end chan error) (*PostgresKeeper, error) {
	storePath := filepath.Join(common.StoreBasePath, cfg.clusterName)

	kvstore, err := store.NewStore(store.Config{
		Backend:       store.Backend(cfg.storeBackend),
		Endpoints:     cfg.storeEndpoints,
		CertFile:      cfg.storeCertFile,
		KeyFile:       cfg.storeKeyFile,
		CAFile:        cfg.storeCAFile,
		SkipTLSVerify: cfg.storeSkipTlsVerify,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}
	e := store.NewStoreManager(kvstore, storePath)

	// Clean and get absolute datadir path
	dataDir, err := filepath.Abs(cfg.dataDir)
	if err != nil {
		return nil, fmt.Errorf("cannot get absolute datadir path for %q: %v", cfg.dataDir, err)
	}

	p := &PostgresKeeper{
		cfg: cfg,

		bootUUID: common.UUID(),

		dataDir:        dataDir,
		storeBackend:   cfg.storeBackend,
		storeEndpoints: cfg.storeEndpoints,

		pgListenAddress:     cfg.pgListenAddress,
		pgPort:              cfg.pgPort,
		pgBinPath:           cfg.pgBinPath,
		pgReplUsername:      cfg.pgReplUsername,
		pgReplPassword:      cfg.pgReplPassword,
		pgSUUsername:        cfg.pgSUUsername,
		pgSUPassword:        cfg.pgSUPassword,
		pgInitialSUUsername: cfg.pgInitialSUUsername,

		sleepInterval:  cluster.DefaultSleepInterval,
		requestTimeout: cluster.DefaultRequestTimeout,

		keeperLocalState: &KeeperLocalState{},
		dbLocalState:     &DBLocalState{},

		e:    e,
		stop: stop,
		end:  end,
	}

	err = p.loadKeeperLocalState()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load keeper local state file: %v", err)
	}
	if p.keeperLocalState.UID != "" && p.cfg.uid != "" && p.keeperLocalState.UID != p.cfg.uid {
		die("saved uid %q differs from configuration uid: %q", p.keeperLocalState.UID, cfg.uid)
	}
	if p.keeperLocalState.UID == "" {
		p.keeperLocalState.UID = cfg.uid
		if cfg.uid == "" {
			p.keeperLocalState.UID = common.UID()
			log.Infow("uid generated", "uid", p.keeperLocalState.UID)
		}
		if err = p.saveKeeperLocalState(); err != nil {
			die("error: %v", err)
		}
	}

	log.Infow("keeper uid", "uid", p.keeperLocalState.UID)

	err = p.loadDBLocalState()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load db local state file: %v", err)
	}
	return p, nil
}

func (p *PostgresKeeper) usePgrewind(db *cluster.DB) bool {
	return p.pgSUUsername != "" && p.pgSUPassword != "" && db.Spec.UsePgrewind
}

func (p *PostgresKeeper) updateKeeperInfo() error {
	p.localStateMutex.Lock()
	keeperUID := p.keeperLocalState.UID
	clusterUID := p.keeperLocalState.ClusterUID
	p.localStateMutex.Unlock()

	if clusterUID == "" {
		return nil
	}

	keeperInfo := &cluster.KeeperInfo{
		InfoUID:       common.UID(),
		UID:           keeperUID,
		ClusterUID:    clusterUID,
		BootUUID:      p.bootUUID,
		PostgresState: p.getLastPGState(),
	}

	// The time to live is just to automatically remove old entries, it's
	// not used to determine if the keeper info has been updated.
	if err := p.e.SetKeeperInfo(keeperUID, keeperInfo, p.sleepInterval); err != nil {
		return err
	}
	return nil
}

func (p *PostgresKeeper) updatePGState(pctx context.Context) {
	p.pgStateMutex.Lock()
	defer p.pgStateMutex.Unlock()
	pgState, err := p.GetPGState(pctx)
	if err != nil {
		log.Errorw("failed to get pg state", zap.Error(err))
		return
	}
	p.lastPGState = pgState
}

// parseSynchronousStandbyNames extracts the standby names from the
// "synchronous_standby_names" postgres parameter.
//
// Since postgres 9.6 (https://www.postgresql.org/docs/9.6/static/runtime-config-replication.html)
// `synchronous_standby_names` can be in one of two formats:
//   num_sync ( standby_name [, ...] )
//   standby_name [, ...]
// two examples for this:
//   2 (node1,node2)
//   node1,node2
// TODO(sgotti) since postgres 10 (https://www.postgresql.org/docs/10/static/runtime-config-replication.html)
// `synchronous_standby_names` can be in one of three formats:
//   [FIRST] num_sync ( standby_name [, ...] )
//   ANY num_sync ( standby_name [, ...] )
//   standby_name [, ...]
// since we are writing ourself the synchronous_standby_names we don't handle this case.
// If needed, to better handle all the cases with also a better validation of
// standby names we could use something like the parser used by postgres
func parseSynchronousStandbyNames(s string) ([]string, error) {
	var spacesSplit []string = strings.Split(s, " ")
	var entries []string
	if len(spacesSplit) < 2 {
		// We're parsing format: standby_name [, ...]
		entries = strings.Split(s, ",")
	} else {
		// We don't know yet which of the 2 formats we're parsing
		_, err := strconv.Atoi(spacesSplit[0])
		if err == nil {
			// We're parsing format: num_sync ( standby_name [, ...] )
			rest := strings.Join(spacesSplit[1:], " ")
			inBrackets := strings.TrimSpace(rest)
			if !strings.HasPrefix(inBrackets, "(") || !strings.HasSuffix(inBrackets, ")") {
				return nil, fmt.Errorf("synchronous standby string has number but lacks brackets")
			}
			withoutBrackets := strings.TrimRight(strings.TrimLeft(inBrackets, "("), ")")
			entries = strings.Split(withoutBrackets, ",")
		} else {
			// We're parsing format: standby_name [, ...]
			entries = strings.Split(s, ",")
		}
	}
	for i, e := range entries {
		entries[i] = strings.TrimSpace(e)
	}
	return entries, nil
}

func (p *PostgresKeeper) GetPGState(pctx context.Context) (*cluster.PostgresState, error) {
	p.getPGStateMutex.Lock()
	defer p.getPGStateMutex.Unlock()
	// Just get one pgstate at a time to avoid exausting available connections
	pgState := &cluster.PostgresState{}

	p.localStateMutex.Lock()
	pgState.UID = p.dbLocalState.UID
	pgState.Generation = p.dbLocalState.Generation
	p.localStateMutex.Unlock()

	pgState.ListenAddress = p.pgListenAddress
	pgState.Port = p.pgPort

	initialized, err := p.pgm.IsInitialized()
	if err != nil {
		return nil, err
	}
	if initialized {
		pgParameters, err := p.pgm.GetConfigFilePGParameters()
		if err != nil {
			log.Errorw("cannot get configured pg parameters", zap.Error(err))
			return pgState, nil
		}
		log.Debugw("got configured pg parameters", "pgParameters", pgParameters)
		filteredPGParameters := common.Parameters{}
		for k, v := range pgParameters {
			if !util.StringInSlice(managedPGParameters, k) {
				filteredPGParameters[k] = v
			}
		}
		log.Debugw("filtered out managed pg parameters", "filteredPGParameters", filteredPGParameters)
		pgState.PGParameters = filteredPGParameters

		synchronousStandbyNames, err := parseSynchronousStandbyNames(pgParameters["synchronous_standby_names"])
		if err != nil {
			log.Errorw("error parsing synchronous_standby_names", zap.Error(err))
			return pgState, nil
		}
		synchronousStandbys := []string{}
		for _, n := range synchronousStandbyNames {
			if !common.IsStolonName(n) {
				continue
			}
			synchronousStandbys = append(synchronousStandbys, common.NameFromStolonName(n))
		}
		pgState.SynchronousStandbys = synchronousStandbys

		sd, err := p.pgm.GetSystemData()
		if err != nil {
			log.Errorw("error getting pg state", zap.Error(err))
			return pgState, nil
		}
		pgState.SystemID = sd.SystemID
		pgState.TimelineID = sd.TimelineID
		pgState.XLogPos = sd.XLogPos

		// if timeline <= 1 then no timeline history file exists.
		pgState.TimelinesHistory = cluster.PostgresTimelinesHistory{}
		if pgState.TimelineID > 1 {
			var tlsh []*postgresql.TimelineHistory
			tlsh, err = p.pgm.GetTimelinesHistory(pgState.TimelineID)
			if err != nil {
				log.Errorw("error getting timeline history", zap.Error(err))
				return pgState, nil
			}
			ctlsh := cluster.PostgresTimelinesHistory{}

			for _, tlh := range tlsh {
				ctlh := &cluster.PostgresTimelineHistory{
					TimelineID:  tlh.TimelineID,
					SwitchPoint: tlh.SwitchPoint,
					Reason:      tlh.Reason,
				}
				ctlsh = append(ctlsh, ctlh)
			}
			pgState.TimelinesHistory = ctlsh
		}

		ow, err := p.pgm.OlderWalFile()
		if err != nil {
			log.Warnw("error getting older wal file", zap.Error(err))
		} else {
			log.Debugw("older wal file", "filename", ow)
			pgState.OlderWalFile = ow
		}
		pgState.Healthy = true
	}

	return pgState, nil
}

func (p *PostgresKeeper) getLastPGState() *cluster.PostgresState {
	p.pgStateMutex.Lock()
	pgState := p.lastPGState.DeepCopy()
	p.pgStateMutex.Unlock()
	log.Debugf("pgstate dump: %s", spew.Sdump(pgState))
	return pgState
}

func (p *PostgresKeeper) Start() {
	endSMCh := make(chan struct{})
	endPgStatecheckerCh := make(chan struct{})
	endUpdateKeeperInfo := make(chan struct{})

	var err error
	var cd *cluster.ClusterData
	cd, _, err = p.e.GetClusterData()
	if err != nil {
		log.Errorw("error retrieving cluster data", zap.Error(err))
	} else if cd != nil {
		if cd.FormatVersion != cluster.CurrentCDFormatVersion {
			log.Errorw("unsupported clusterdata format version", "version", cd.FormatVersion)
		} else if cd.Cluster != nil {
			p.sleepInterval = cd.Cluster.DefSpec().SleepInterval.Duration
			p.requestTimeout = cd.Cluster.DefSpec().RequestTimeout.Duration
		}
	}

	log.Debugf("cd dump: %s", spew.Sdump(cd))

	// TODO(sgotti) reconfigure the various configurations options
	// (RequestTimeout) after a changed cluster config
	pgParameters := make(common.Parameters)
	pgm := postgresql.NewManager(p.pgBinPath, p.dataDir, pgParameters, p.getLocalConnParams(), p.getLocalReplConnParams(), p.pgSUUsername, p.pgSUPassword, p.pgReplUsername, p.pgReplPassword, p.requestTimeout)
	p.pgm = pgm

	p.pgm.Stop(true)

	ctx, cancel := context.WithCancel(context.Background())
	smTimerCh := time.NewTimer(0).C
	updatePGStateTimerCh := time.NewTimer(0).C
	updateKeeperInfoTimerCh := time.NewTimer(0).C
	for true {
		select {
		case <-p.stop:
			log.Debugw("stopping stolon keeper")
			cancel()
			p.pgm.Stop(true)
			p.end <- nil
			return

		case <-smTimerCh:
			go func() {
				p.postgresKeeperSM(ctx)
				endSMCh <- struct{}{}
			}()

		case <-endSMCh:
			smTimerCh = time.NewTimer(p.sleepInterval).C

		case <-updatePGStateTimerCh:
			// updateKeeperInfo two times faster than the sleep interval
			go func() {
				p.updatePGState(ctx)
				endPgStatecheckerCh <- struct{}{}
			}()

		case <-endPgStatecheckerCh:
			// updateKeeperInfo two times faster than the sleep interval
			updatePGStateTimerCh = time.NewTimer(p.sleepInterval / 2).C

		case <-updateKeeperInfoTimerCh:
			go func() {
				if err := p.updateKeeperInfo(); err != nil {
					log.Errorw("failed to update keeper info", zap.Error(err))
				}
				endUpdateKeeperInfo <- struct{}{}
			}()

		case <-endUpdateKeeperInfo:
			updateKeeperInfoTimerCh = time.NewTimer(p.sleepInterval).C
		}
	}
}

func (p *PostgresKeeper) resync(db, followedDB *cluster.DB, tryPgrewind bool) error {
	pgm := p.pgm
	replConnParams := p.getReplConnParams(db, followedDB)
	standbySettings := &cluster.StandbySettings{PrimaryConninfo: replConnParams.ConnString(), PrimarySlotName: common.StolonName(db.UID)}

	// TODO(sgotti) Actually we don't check if pg_rewind is installed or if
	// postgresql version is > 9.5 since someone can also use an externally
	// installed pg_rewind for postgres 9.4. If a pg_rewind executable
	// doesn't exists pgm.SyncFromFollowedPGRewind will return an error and
	// fallback to pg_basebackup
	if tryPgrewind && p.usePgrewind(db) {
		connParams := p.getSUConnParams(db, followedDB)
		log.Infow("syncing using pg_rewind", "followedDB", followedDB.UID, "keeper", followedDB.Spec.KeeperUID)
		if err := pgm.SyncFromFollowedPGRewind(connParams, p.pgSUPassword); err != nil {
			// log pg_rewind error and fallback to pg_basebackup
			log.Errorw("error syncing with pg_rewind", zap.Error(err))
		} else {
			if err := pgm.WriteRecoveryConf(p.createRecoveryParameters(standbySettings, nil, nil)); err != nil {
				return fmt.Errorf("err: %v", err)
			}
			return nil
		}
	}

	if err := pgm.RemoveAll(); err != nil {
		return fmt.Errorf("failed to remove the postgres data dir: %v", err)
	}
	if slog.IsDebug() {
		log.Debugw("syncing from followed db", "followedDB", followedDB.UID, "keeper", followedDB.Spec.KeeperUID, "replConnParams", fmt.Sprintf("%v", replConnParams))
	} else {
		log.Infow("syncing from followed db", "followedDB", followedDB.UID, "keeper", followedDB.Spec.KeeperUID)
	}
	if err := pgm.SyncFromFollowed(replConnParams); err != nil {
		return fmt.Errorf("sync error: %v", err)
	}
	log.Infow("sync succeeded")

	if err := pgm.WriteRecoveryConf(p.createRecoveryParameters(standbySettings, nil, nil)); err != nil {
		return fmt.Errorf("err: %v", err)
	}
	return nil
}

// TODO(sgotti) unify this with the sentinel one. They have the same logic but one uses *cluster.PostgresState while the other *cluster.DB
func (p *PostgresKeeper) isDifferentTimelineBranch(followedDB *cluster.DB, pgState *cluster.PostgresState) bool {
	if followedDB.Status.TimelineID < pgState.TimelineID {
		log.Infow("followed instance timeline < than our timeline", "followedTimeline", followedDB.Status.TimelineID, "timeline", pgState.TimelineID)
		return true
	}

	// if the timelines are the same check that also the switchpoints are the same.
	if followedDB.Status.TimelineID == pgState.TimelineID {
		if pgState.TimelineID <= 1 {
			// if timeline <= 1 then no timeline history file exists.
			return false
		}
		ftlh := followedDB.Status.TimelinesHistory.GetTimelineHistory(pgState.TimelineID - 1)
		tlh := pgState.TimelinesHistory.GetTimelineHistory(pgState.TimelineID - 1)
		if ftlh == nil || tlh == nil {
			// No timeline history to check
			return false
		}
		if ftlh.SwitchPoint == tlh.SwitchPoint {
			return false
		}
		log.Infow("followed instance timeline forked at a different xlog pos than our timeline", "followedTimeline", followedDB.Status.TimelineID, "followedXlogpos", ftlh.SwitchPoint, "timeline", pgState.TimelineID, "xlogpos", tlh.SwitchPoint)
		return true
	}

	// followedDB.Status.TimelineID > pgState.TimelineID
	ftlh := followedDB.Status.TimelinesHistory.GetTimelineHistory(pgState.TimelineID)
	if ftlh != nil {
		if ftlh.SwitchPoint < pgState.XLogPos {
			log.Infow("followed instance timeline forked before our current state", "followedTimeline", followedDB.Status.TimelineID, "followedXlogpos", ftlh.SwitchPoint, "timeline", pgState.TimelineID, "xlogpos", pgState.XLogPos)
			return true
		}
	}
	return false
}

func (p *PostgresKeeper) updateReplSlots(dbLocalState *DBLocalState, followersUIDs []string) error {
	var replSlots []string
	replSlots, err := p.pgm.GetReplicationSlots()
	log.Debugw("replication slots", "replSlots", replSlots)
	if err != nil {
		log.Errorw("failed to get replication slots", zap.Error(err))
		return err
	}
	// Drop replication slots
	for _, slotName := range replSlots {
		if !common.IsStolonName(slotName) {
			continue
		}
		if !util.StringInSlice(followersUIDs, common.NameFromStolonName(slotName)) {
			log.Infow("dropping replication slot since db not marked as follower", "slot", slotName, "db", common.NameFromStolonName(slotName))
			if err = p.pgm.DropReplicationSlot(slotName); err != nil {
				log.Errorw("failed to drop replication slot", "slotName", slotName, "err", err)
				return err
			}
		}
	}
	// Create replication slots
	for _, followerUID := range followersUIDs {
		if followerUID == dbLocalState.UID {
			continue
		}
		replSlot := common.StolonName(followerUID)
		if !util.StringInSlice(replSlots, replSlot) {
			log.Infow("creating replication slot", "slot", replSlot, "db", followerUID)
			if err = p.pgm.CreateReplicationSlot(replSlot); err != nil {
				log.Errorw("failed to create replication slot", "slotName", replSlot, zap.Error(err))
				return err
			}
		}
	}
	return nil
}

func (p *PostgresKeeper) postgresKeeperSM(pctx context.Context) {
	e := p.e
	pgm := p.pgm

	cd, _, err := e.GetClusterData()
	if err != nil {
		log.Errorw("error retrieving cluster data", zap.Error(err))
		return
	}
	log.Debugf("cd dump: %s", spew.Sdump(cd))

	if cd == nil {
		log.Infow("no cluster data available, waiting for it to appear")
		return
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		log.Errorw("unsupported clusterdata format version", "version", cd.FormatVersion)
		return
	}
	if err = cd.Cluster.Spec.Validate(); err != nil {
		log.Errorw("clusterdata validation failed", zap.Error(err))
		return
	}
	if cd.Cluster != nil {
		p.sleepInterval = cd.Cluster.DefSpec().SleepInterval.Duration
		p.requestTimeout = cd.Cluster.DefSpec().RequestTimeout.Duration

		if p.keeperLocalState.ClusterUID != cd.Cluster.UID {
			p.keeperLocalState.ClusterUID = cd.Cluster.UID
			if err = p.saveKeeperLocalState(); err != nil {
				log.Errorw("failed to save keeper local state", zap.Error(err))
				return
			}
		}
	}

	k, ok := cd.Keepers[p.keeperLocalState.UID]
	if !ok {
		log.Infow("our keeper data is not available, waiting for it to appear")
		return
	}

	db := cd.FindDB(k)
	if db == nil {
		log.Infow("no db assigned")
		return
	}

	if p.bootUUID != k.Status.BootUUID {
		log.Infow("our db boot UID is different than the cluster data one, waiting for it to be updated", "bootUUID", p.bootUUID, "clusterBootUUID", k.Status.BootUUID)
		pgm.Stop(true)
		return
	}

	followersUIDs := db.Spec.Followers

	prevPGParameters := pgm.GetParameters()
	var pgParameters common.Parameters

	dbls := p.dbLocalState
	if dbls.Initializing {
		// If we are here this means that the db initialization or
		// resync has failed so we have to clean up stale data
		log.Errorw("db failed to initialize or resync")

		pgm.Stop(true)

		// Clean up cluster db datadir
		if err = pgm.RemoveAll(); err != nil {
			log.Errorw("failed to remove the postgres data dir", zap.Error(err))
			return
		}
		// Reset current db local state since it's not valid anymore
		p.localStateMutex.Lock()
		dbls.UID = ""
		dbls.Generation = cluster.NoGeneration
		dbls.Initializing = false
		p.localStateMutex.Unlock()
		if err = p.saveDBLocalState(); err != nil {
			log.Errorw("failed to save db local state", zap.Error(err))
			return
		}
	}

	initialized, err := pgm.IsInitialized()
	if err != nil {
		log.Errorw("failed to detect if instance is initialized", zap.Error(err))
		return
	}

	started := false
	if initialized {
		started, err = pgm.IsStarted()
		if err != nil {
			// log error getting instance state but go ahead.
			log.Errorw("failed to retrieve instance status", zap.Error(err))
		}
	}

	log.Debugw("db status", "started", started)

	// if the db is initialized but there isn't a db local state then generate a new one
	if initialized && dbls.UID == "" {
		p.localStateMutex.Lock()
		dbls.UID = common.UID()
		dbls.Generation = cluster.NoGeneration
		dbls.InitPGParameters = nil
		dbls.Initializing = false
		p.localStateMutex.Unlock()
		if err = p.saveDBLocalState(); err != nil {
			log.Errorw("failed to save db local state", zap.Error(err))
			return
		}
	}

	if dbls.UID != db.UID {
		log.Infow("current db UID different than cluster data db UID", "db", dbls.UID, "cdDB", db.UID)
		switch db.Spec.InitMode {
		case cluster.DBInitModeNew:
			log.Infow("initializing the database cluster")
			p.localStateMutex.Lock()
			dbls.UID = db.UID
			// Set a no generation since we aren't already converged.
			dbls.Generation = cluster.NoGeneration
			dbls.InitPGParameters = nil
			dbls.Initializing = true
			p.localStateMutex.Unlock()
			if err = p.saveDBLocalState(); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}

			// create postgres parameteres with empty InitPGParameters
			pgParameters = p.createPGParameters(db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if started {
				if err = pgm.Stop(true); err != nil {
					log.Errorw("failed to stop pg instance", zap.Error(err))
					return
				}
				started = false
			}
			if err = pgm.RemoveAll(); err != nil {
				log.Errorw("failed to remove the postgres data dir", zap.Error(err))
				return
			}
			if err = pgm.Init(); err != nil {
				log.Errorw("failed to initialize postgres database cluster", zap.Error(err))
				return
			}
			initialized = true

			if db.Spec.IncludeConfig {
				if err = pgm.StartTmpMerged(); err != nil {
					log.Errorw("failed to start instance", zap.Error(err))
					return
				}
				pgParameters, err = pgm.GetConfigFilePGParameters()
				if err != nil {
					log.Errorw("failed to retrieve postgres parameters", zap.Error(err))
					return
				}
				p.localStateMutex.Lock()
				dbls.InitPGParameters = pgParameters
				p.localStateMutex.Unlock()
			} else {
				if err = pgm.StartTmpMerged(); err != nil {
					log.Errorw("failed to start instance", zap.Error(err))
					return
				}
			}

			log.Infow("setting roles")
			if err = pgm.SetupRoles(); err != nil {
				log.Errorw("failed to setup roles", zap.Error(err))
				return
			}

			if err = p.saveDBLocalState(); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}
			if err = pgm.Stop(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}
		case cluster.DBInitModePITR:
			log.Infow("restoring the database cluster")
			p.localStateMutex.Lock()
			dbls.UID = db.UID
			// Set a no generation since we aren't already converged.
			dbls.Generation = cluster.NoGeneration
			dbls.InitPGParameters = nil
			dbls.Initializing = true
			p.localStateMutex.Unlock()
			if err = p.saveDBLocalState(); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}

			// create postgres parameteres with empty InitPGParameters
			pgParameters = p.createPGParameters(db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if started {
				if err = pgm.Stop(true); err != nil {
					log.Errorw("failed to stop pg instance", zap.Error(err))
					return
				}
				started = false
			}
			if err = pgm.RemoveAll(); err != nil {
				log.Errorw("failed to remove the postgres data dir", zap.Error(err))
				return
			}
			log.Infow("executing DataRestoreCommand")
			if err = pgm.Restore(db.Spec.PITRConfig.DataRestoreCommand); err != nil {
				log.Errorw("failed to restore postgres database cluster", zap.Error(err))
				return
			}
			var standbySettings *cluster.StandbySettings
			if db.Spec.FollowConfig != nil && db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
				standbySettings = db.Spec.FollowConfig.StandbySettings
			}
			if err = pgm.WriteRecoveryConf(p.createRecoveryParameters(standbySettings, db.Spec.PITRConfig.ArchiveRecoverySettings, db.Spec.PITRConfig.RecoveryTargetSettings)); err != nil {
				log.Errorw("error writing recovery.conf", zap.Error(err))
				return
			}
			if err = pgm.StartTmpMerged(); err != nil {
				log.Errorw("failed to start instance", zap.Error(err))
				return
			}
			// wait for the db having replyed all the wals
			if err = pgm.WaitReady(cd.Cluster.DefSpec().SyncTimeout.Duration); err != nil {
				log.Errorw("instance not ready", zap.Error(err))
				return
			}

			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters()
				if err != nil {
					log.Errorw("failed to retrieve postgres parameters", zap.Error(err))
					return
				}
				p.localStateMutex.Lock()
				dbls.InitPGParameters = pgParameters
				p.localStateMutex.Unlock()
			}
			initialized = true

			if err = p.saveDBLocalState(); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}
			if err = pgm.Stop(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}
		case cluster.DBInitModeResync:
			log.Infow("resyncing the database cluster")
			// replace our current db uid with the required one.
			p.localStateMutex.Lock()
			dbls.UID = db.UID
			// Set a no generation since we aren't already converged.
			dbls.Generation = cluster.NoGeneration
			dbls.InitPGParameters = nil
			dbls.Initializing = true
			p.localStateMutex.Unlock()
			if err = p.saveDBLocalState(); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}

			if started {
				if err = pgm.Stop(true); err != nil {
					log.Errorw("failed to stop pg instance", zap.Error(err))
					return
				}
				started = false
			}

			// create postgres parameteres with empty InitPGParameters
			pgParameters = p.createPGParameters(db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			var systemID string
			if !initialized {
				log.Infow("database cluster not initialized")
			} else {
				systemID, err = pgm.GetSystemdID()
				if err != nil {
					log.Errorw("error retrieving systemd ID", zap.Error(err))
					return
				}
			}

			followedUID := db.Spec.FollowConfig.DBUID
			followedDB, ok := cd.DBs[followedUID]
			if !ok {
				log.Errorw("no db data available for followed db", "followedDB", followedUID)
				return
			}

			tryPgrewind := true
			if !initialized {
				tryPgrewind = false
			}
			if systemID != followedDB.Status.SystemID {
				tryPgrewind = false
			}

			// TODO(sgotti) pg_rewind considers databases on the same timeline
			// as in sync and doesn't check if they diverged at different
			// position in previous timelines.
			// So check that the db as been synced or resync again with
			// pg_rewind disabled. Will need to report this upstream.

			// TODO(sgotti) The rewinded standby needs wal from the master
			// starting from the common ancestor, if they aren't available the
			// instance will keep waiting for them, now we assume that if the
			// instance isn't ready after the start timeout, it's waiting for
			// wals and we'll force a full resync.
			// We have to find a better way to detect if a standby is waiting
			// for unavailable wals.
			if err = p.resync(db, followedDB, tryPgrewind); err != nil {
				log.Errorw("failed to resync from followed instance", zap.Error(err))
				return
			}
			if err = pgm.Start(); err != nil {
				log.Errorw("err", zap.Error(err))
				return
			}
			started = true

			if tryPgrewind {
				fullResync := false
				// if not accepting connection assume that it's blocked waiting for missing wal
				// (see above TODO), so do a full resync using pg_basebackup.
				if err = pgm.Ping(); err != nil {
					log.Errorw("pg_rewinded standby is not accepting connection. it's probably waiting for unavailable wals. Forcing a full resync")
					fullResync = true
				} else {
					// Check again if it was really synced
					var pgState *cluster.PostgresState
					pgState, err = p.GetPGState(pctx)
					if err != nil {
						log.Errorw("cannot get current pgstate", zap.Error(err))
						return
					}
					if p.isDifferentTimelineBranch(followedDB, pgState) {
						fullResync = true
					}
				}

				if fullResync {
					if started {
						if err = pgm.Stop(true); err != nil {
							log.Errorw("failed to stop pg instance", zap.Error(err))
							return
						}
						started = false
					}
					if err = p.resync(db, followedDB, false); err != nil {
						log.Errorw("failed to resync from followed instance", zap.Error(err))
						return
					}
				}
			}
			initialized = true

		case cluster.DBInitModeExisting:
			// replace our current db uid with the required one.
			p.localStateMutex.Lock()
			dbls.UID = db.UID
			// Set a no generation since we aren't already converged.
			dbls.Generation = cluster.NoGeneration
			dbls.InitPGParameters = nil
			p.localStateMutex.Unlock()
			if err = p.saveDBLocalState(); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}

			// create postgres parameteres with empty InitPGParameters
			pgParameters = p.createPGParameters(db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if started {
				if err = pgm.Stop(true); err != nil {
					log.Errorw("failed to stop pg instance", zap.Error(err))
					return
				}
				started = false
			}
			if db.Spec.IncludeConfig {
				if err = pgm.StartTmpMerged(); err != nil {
					log.Errorw("failed to start instance", zap.Error(err))
					return
				}
				pgParameters, err = pgm.GetConfigFilePGParameters()
				if err != nil {
					log.Errorw("failed to retrieve postgres parameters", zap.Error(err))
					return
				}
				p.localStateMutex.Lock()
				dbls.InitPGParameters = pgParameters
				p.localStateMutex.Unlock()
			} else {
				if err = pgm.StartTmpMerged(); err != nil {
					log.Errorw("failed to start instance", zap.Error(err))
					return
				}
			}
			log.Infow("updating our db UID with the cluster data provided db UID")
			// replace our current db uid with the required one.
			p.localStateMutex.Lock()
			dbls.InitPGParameters = pgParameters
			p.localStateMutex.Unlock()
			if err = p.saveDBLocalState(); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}
			if err = pgm.Stop(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}
		case cluster.DBInitModeNone:
			// replace our current db uid with the required one.
			p.localStateMutex.Lock()
			dbls.UID = db.UID
			// Set a no generation since we aren't already converged.
			dbls.Generation = cluster.NoGeneration
			dbls.InitPGParameters = nil
			p.localStateMutex.Unlock()
			if err = p.saveDBLocalState(); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}
			return
		default:
			log.Errorw("unknown db init mode", "initMode", string(db.Spec.InitMode))
			return
		}
	}

	// create postgres parameteres
	pgParameters = p.createPGParameters(db)
	// update pgm postgres parameters
	pgm.SetParameters(pgParameters)

	var localRole common.Role
	if !initialized {
		log.Infow("database cluster not initialized")
		localRole = common.RoleUndefined
	} else {
		localRole, err = pgm.GetRole()
		if err != nil {
			log.Errorw("error retrieving current pg role", zap.Error(err))
			return
		}
	}

	targetRole := db.Spec.Role
	log.Debugw("target role", "targetRole", string(targetRole))

	switch targetRole {
	case common.RoleMaster:
		// We are the elected master
		log.Infow("our db requested role is master")
		if localRole == common.RoleUndefined {
			log.Errorw("database cluster not initialized but requested role is master. This shouldn't happen!")
			return
		}
		if !started {
			if err = pgm.Start(); err != nil {
				log.Errorw("failed to start postgres", zap.Error(err))
				return
			}
			started = true
		}

		if localRole == common.RoleStandby {
			log.Infow("promoting to master")
			if err = pgm.Promote(); err != nil {
				log.Errorw("failed to promote instance", zap.Error(err))
				return
			}
		} else {
			log.Infow("already master")
		}

		if err = p.updateReplSlots(dbls, followersUIDs); err != nil {
			log.Errorw("error updating replication slots", zap.Error(err))
			return
		}

	case common.RoleStandby:
		// We are a standby
		var standbySettings *cluster.StandbySettings
		switch db.Spec.FollowConfig.Type {
		case cluster.FollowTypeInternal:
			followedUID := db.Spec.FollowConfig.DBUID
			log.Infow("our db requested role is standby", "followedDB", followedUID)
			followedDB, ok := cd.DBs[followedUID]
			if !ok {
				log.Errorw("no db data available for followed db", "followedDB", followedUID)
				return
			}
			replConnParams := p.getReplConnParams(db, followedDB)
			standbySettings = &cluster.StandbySettings{PrimaryConninfo: replConnParams.ConnString(), PrimarySlotName: common.StolonName(db.UID)}
		case cluster.FollowTypeExternal:
			standbySettings = db.Spec.FollowConfig.StandbySettings
		default:
			log.Errorw("unknown follow type", "followType", string(db.Spec.FollowConfig.Type))
			return
		}
		switch localRole {
		case common.RoleMaster:
			log.Errorw("cannot move from master role to standby role")
			return
		case common.RoleStandby:
			log.Infow("already standby")
			if !started {
				if err = pgm.WriteRecoveryConf(p.createRecoveryParameters(standbySettings, nil, nil)); err != nil {
					log.Errorw("error writing recovery.conf", zap.Error(err))
					return
				}
				if err = pgm.Start(); err != nil {
					log.Errorw("failed to start postgres", zap.Error(err))
					return
				}
				started = true
			}

			// TODO(sgotti) Check that the followed instance has all the needed WAL segments

			// Update our primary_conninfo if replConnString changed
			if db.Spec.FollowConfig.Type == cluster.FollowTypeInternal {
				var curReplConnParams postgresql.ConnParams

				curReplConnParams, err = pgm.GetPrimaryConninfo()
				if err != nil {
					log.Errorw("failed to get postgres primary conn info", zap.Error(err))
					return
				}
				log.Debugw("curReplConnParams", "curReplConnParams", curReplConnParams)

				followedUID := db.Spec.FollowConfig.DBUID
				followedDB, ok := cd.DBs[followedUID]
				if !ok {
					log.Errorw("no db data available for followed db", "followedDB", followedUID)
					return
				}
				newReplConnParams := p.getReplConnParams(db, followedDB)
				log.Debugw("newReplConnParams", "newReplConnParams", newReplConnParams)

				if !curReplConnParams.Equals(newReplConnParams) {
					log.Infow("connection parameters changed. Reconfiguring.", "followedDB", followedUID, "replConnParams", newReplConnParams)
					standbySettings := &cluster.StandbySettings{PrimaryConninfo: newReplConnParams.ConnString(), PrimarySlotName: common.StolonName(db.UID)}
					if err = pgm.WriteRecoveryConf(p.createRecoveryParameters(standbySettings, nil, nil)); err != nil {
						log.Errorw("error writing recovery.conf", zap.Error(err))
						return
					}
					if err = pgm.Restart(true); err != nil {
						log.Errorw("failed to restart postgres instance", zap.Error(err))
						return
					}
				}
			}

			if db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
				// Update recovery conf if our FollowConfig has changed
				curReplConnParams, err := pgm.GetPrimaryConninfo()
				if err != nil {
					log.Errorw("failed to get postgres primary conn info", zap.Error(err))
					return
				}
				log.Debugw("curReplConnParams", "curReplConnParams", curReplConnParams)

				newReplConnParams, err := pg.ParseConnString(db.Spec.FollowConfig.StandbySettings.PrimaryConninfo)
				if err != nil {
					log.Errorw("failed ot parse standby connection string", zap.Error(err))
					return
				}
				log.Debugw("newReplConnParams", "newReplConnParams", newReplConnParams)

				curPrimarySlotName, err := pgm.GetPrimarySlotName()
				if err != nil {
					log.Errorw("failed to get current primary slot name", zap.Error(err))
					return
				}

				if !curReplConnParams.Equals(newReplConnParams) || curPrimarySlotName != db.Spec.FollowConfig.StandbySettings.PrimarySlotName {
					standbySettings := db.Spec.FollowConfig.StandbySettings
					if err = pgm.WriteRecoveryConf(p.createRecoveryParameters(standbySettings, nil, nil)); err != nil {
						log.Errorw("error writing recovery.conf", zap.Error(err))
						return
					}
					if err = pgm.Restart(true); err != nil {
						log.Errorw("failed to restart postgres instance", zap.Error(err))
						return
					}
				}

				if err = p.updateReplSlots(dbls, followersUIDs); err != nil {
					log.Errorw("error updating replication slots", zap.Error(err))
					return
				}
			}

		case common.RoleUndefined:
			log.Infow("our db role is none")
			return
		}
	case common.RoleUndefined:
		log.Infow("our db requested role is none")
		return
	}

	// update pg parameters
	pgParameters = p.createPGParameters(db)

	// Log synchronous replication changes
	prevSyncStandbyNames := prevPGParameters["synchronous_standby_names"]
	syncStandbyNames := pgParameters["synchronous_standby_names"]
	if db.Spec.SynchronousReplication {
		if prevSyncStandbyNames != syncStandbyNames {
			log.Infow("needed synchronous_standby_names changed", "prevSyncStandbyNames", prevSyncStandbyNames, "syncStandbyNames", syncStandbyNames)
		}
	} else {
		if prevSyncStandbyNames != "" {
			log.Infow("sync replication disabled, removing current synchronous_standby_names", "syncStandbyNames", prevSyncStandbyNames)
		}
	}

	if !pgParameters.Equals(prevPGParameters) {
		log.Infow("postgres parameters changed, reloading postgres instance")
		pgm.SetParameters(pgParameters)
		if err := pgm.Reload(); err != nil {
			log.Errorw("failed to reload postgres instance", err)
		}
	} else {
		// for tests
		log.Infow("postgres parameters not changed")
	}

	// If we are here, then all went well and we can update the db generation and save it locally
	p.localStateMutex.Lock()
	dbls.Generation = db.Generation
	dbls.Initializing = false
	p.localStateMutex.Unlock()
	if err := p.saveDBLocalState(); err != nil {
		log.Errorw("failed to save db local state", zap.Error(err))
		return
	}
}

func (p *PostgresKeeper) keeperLocalStateFilePath() string {
	return filepath.Join(p.cfg.dataDir, "keeperstate")
}

func (p *PostgresKeeper) loadKeeperLocalState() error {
	sj, err := ioutil.ReadFile(p.keeperLocalStateFilePath())
	if err != nil {
		return err
	}
	var s *KeeperLocalState
	if err := json.Unmarshal(sj, &s); err != nil {
		return err
	}
	p.keeperLocalState = s
	return nil
}

func (p *PostgresKeeper) saveKeeperLocalState() error {
	sj, err := json.Marshal(p.keeperLocalState)
	if err != nil {
		return err
	}
	return common.WriteFileAtomic(p.keeperLocalStateFilePath(), sj, 0600)
}

func (p *PostgresKeeper) dbLocalStateFilePath() string {
	return filepath.Join(p.cfg.dataDir, "dbstate")
}

func (p *PostgresKeeper) loadDBLocalState() error {
	sj, err := ioutil.ReadFile(p.dbLocalStateFilePath())
	if err != nil {
		return err
	}
	var s *DBLocalState
	if err := json.Unmarshal(sj, &s); err != nil {
		return err
	}
	p.dbLocalState = s
	return nil
}

func (p *PostgresKeeper) saveDBLocalState() error {
	sj, err := json.Marshal(p.dbLocalState)
	if err != nil {
		return err
	}
	return common.WriteFileAtomic(p.dbLocalStateFilePath(), sj, 0600)
}

func sigHandler(sigs chan os.Signal, stop chan bool) {
	s := <-sigs
	log.Debugw("got signal", "signal", s)
	close(stop)
}

func main() {
	flagutil.SetFlagsFromEnv(cmdKeeper.PersistentFlags(), "STKEEPER")

	cmdKeeper.Execute()
}

func keeper(cmd *cobra.Command, args []string) {
	var err error
	switch cfg.logLevel {
	case "error":
		slog.SetLevel(zap.ErrorLevel)
	case "warn":
		slog.SetLevel(zap.WarnLevel)
	case "info":
		slog.SetLevel(zap.InfoLevel)
	case "debug":
		slog.SetLevel(zap.DebugLevel)
	default:
		die("invalid log level: %v", cfg.logLevel)
	}
	if cfg.debug {
		slog.SetDebug()
	}

	if cfg.dataDir == "" {
		die("data dir required")
	}

	if cfg.clusterName == "" {
		die("cluster name required")
	}
	if cfg.storeBackend == "" {
		die("store backend type required")
	}

	if err = os.MkdirAll(cfg.dataDir, 0700); err != nil {
		die("cannot create data dir: %v", err)
	}

	if cfg.pgReplUsername == "" {
		die("--pg-repl-username is required")
	}

	if cfg.pgReplPassword == "" && cfg.pgReplPasswordFile == "" {
		die("one of --pg-repl-password or --pg-repl-passwordfile is required")
	}
	if cfg.pgReplPassword != "" && cfg.pgReplPasswordFile != "" {
		die("only one of --pg-repl-password or --pg-repl-passwordfile must be provided")
	}
	if cfg.pgSUPassword == "" && cfg.pgSUPasswordFile == "" {
		die("one of --pg-su-password or --pg-su-passwordfile is required")
	}
	if cfg.pgSUPassword != "" && cfg.pgSUPasswordFile != "" {
		die("only one of --pg-su-password or --pg-su-passwordfile must be provided")
	}

	if cfg.pgSUUsername == cfg.pgReplUsername {
		stdout("warning: superuser name and replication user name are the same. Different users are suggested.")
		if cfg.pgSUPassword != cfg.pgReplPassword {
			die("provided superuser name and replication user name are the same but provided passwords are different")
		}
	}

	if cfg.pgReplPasswordFile != "" {
		cfg.pgReplPassword, err = readPasswordFromFile(cfg.pgReplPasswordFile)
		if err != nil {
			die("cannot read pg replication user password: %v", err)
		}
	}
	if cfg.pgSUPasswordFile != "" {
		cfg.pgSUPassword, err = readPasswordFromFile(cfg.pgSUPasswordFile)
		if err != nil {
			die("cannot read pg superuser password: %v", err)
		}
	}

	// Open (and create if needed) the lock file.
	// There is no need to clean up this file since we don't use the file as an actual lock. We get a lock
	// on the file. So the lock get released when our process stops (or dies).
	lockFileName := filepath.Join(cfg.dataDir, "lock")
	lockFile, err := os.OpenFile(lockFileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		die("cannot take exclusive lock on data dir %q: %v", lockFileName, err)
	}

	// Get a lock on our lock file.
	ft := &syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: int16(os.SEEK_SET),
		Start:  0,
		Len:    0, // Entire file.
	}

	err = syscall.FcntlFlock(lockFile.Fd(), syscall.F_SETLK, ft)
	if err != nil {
		die("cannot take exclusive lock on data dir %q: %v", lockFileName, err)
	}

	log.Infow("exclusive lock on data dir taken")

	if cfg.uid != "" {
		if !pg.IsValidReplSlotName(cfg.uid) {
			die("keeper uid %q not valid. It can contain only lower-case letters, numbers and the underscore character", cfg.uid)
		}
	}

	stop := make(chan bool, 0)
	end := make(chan error, 0)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go sigHandler(sigs, stop)

	p, err := NewPostgresKeeper(&cfg, stop, end)
	if err != nil {
		die("cannot create keeper: %v", err)
	}
	go p.Start()

	<-end
}
