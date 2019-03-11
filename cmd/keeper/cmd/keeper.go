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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mitchellh/copystructure"
	"github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/common"
	"github.com/sorintlab/stolon/internal/flagutil"
	slog "github.com/sorintlab/stolon/internal/log"
	pg "github.com/sorintlab/stolon/internal/postgresql"
	"github.com/sorintlab/stolon/internal/store"
	"github.com/sorintlab/stolon/internal/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var log = slog.S()

var CmdKeeper = &cobra.Command{
	Use:     "stolon-keeper",
	Run:     keeper,
	Version: cmd.Version,
}

const (
	maxPostgresTimelinesHistory = 2
	minWalKeepSegments          = 8
)

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

func (s *DBLocalState) DeepCopy() *DBLocalState {
	if s == nil {
		return nil
	}
	ns, err := copystructure.Copy(s)
	if err != nil {
		panic(err)
	}
	// paranoid test
	if !reflect.DeepEqual(s, ns) {
		panic("not equal")
	}
	return ns.(*DBLocalState)
}

type config struct {
	cmd.CommonConfig

	uid                string
	dataDir            string
	debug              bool
	priority           int
	prioritySpecified  bool // true iff explicitly set by user
	pgListenAddress    string
	pgAdvertiseAddress string
	pgPort             string
	pgAdvertisePort    string
	pgBinPath          string
	pgReplAuthMethod   string
	pgReplUsername     string
	pgReplPassword     string
	pgReplPasswordFile string
	pgSUAuthMethod     string
	pgSUUsername       string
	pgSUPassword       string
	pgSUPasswordFile   string

	canBeMaster             bool
	canBeSynchronousReplica bool
}

var cfg config

func init() {
	cmd.AddCommonFlags(CmdKeeper, &cfg.CommonConfig)

	CmdKeeper.PersistentFlags().StringVar(&cfg.uid, "id", "", "keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.uid, "uid", "", "keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.dataDir, "data-dir", "", "data directory")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgListenAddress, "pg-listen-address", "", "postgresql instance listening address, local address used for the postgres instance. For all network interface, you can set the value to '*'.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgAdvertiseAddress, "pg-advertise-address", "", "postgresql instance address from outside. Use it to expose ip different than local ip with a NAT networking config")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgPort, "pg-port", "5432", "postgresql instance listening port")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgAdvertisePort, "pg-advertise-port", "", "postgresql instance port from outside. Use it to expose port different than local port with a PAT networking config")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgBinPath, "pg-bin-path", "", "absolute path to postgresql binaries. If empty they will be searched in the current PATH")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplAuthMethod, "pg-repl-auth-method", "md5", "postgres replication user auth method. Default is md5.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplUsername, "pg-repl-username", "", "postgres replication user name. Required. It'll be created on db initialization. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplPassword, "pg-repl-password", "", "postgres replication user password. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplPasswordFile, "pg-repl-passwordfile", "", "postgres replication user password file. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUAuthMethod, "pg-su-auth-method", "md5", "postgres superuser auth method. Default is md5.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUUsername, "pg-su-username", "", "postgres superuser user name. Used for keeper managed instance access and pg_rewind based synchronization. It'll be created on db initialization. Defaults to the name of the effective user running stolon-keeper. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUPassword, "pg-su-password", "", "postgres superuser password. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUPasswordFile, "pg-su-passwordfile", "", "postgres superuser password file. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers)")
	CmdKeeper.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
	CmdKeeper.PersistentFlags().IntVar(&cfg.priority, "priority", 0, "keeper priority, integer. Stolon will promote available keeper with higher priority than current master, if this is possible. Healthy keeper with higher priority will be elected even if current master is online. If not specified, priority is set to "+strconv.Itoa(cluster.DefaultPriority)+" on first keeper invocation; on subsequent invocations, last value (which could be also set with 'stolonctl setkeeperpriority') is reused.")

	CmdKeeper.PersistentFlags().BoolVar(&cfg.canBeMaster, "can-be-master", true, "prevent keeper from being elected as master")
	CmdKeeper.PersistentFlags().BoolVar(&cfg.canBeSynchronousReplica, "can-be-synchronous-replica", true, "prevent keeper from being chosen as synchronous replica")

	if err := CmdKeeper.PersistentFlags().MarkDeprecated("id", "please use --uid"); err != nil {
		log.Fatal(err)
	}
	if err := CmdKeeper.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead"); err != nil {
		log.Fatal(err)
	}
}

var managedPGParameters = []string{
	"unix_socket_directories",
	"wal_keep_segments",
	"hot_standby",
	"listen_addresses",
	"port",
	"max_replication_slots",
	"max_wal_senders",
	"wal_log_hints",
	"synchronous_standby_names",

	// parameters moved from recovery.conf to postgresql.conf in PostgresSQL 12
	"primary_conninfo",
	"primary_slot_name",
	"recovery_min_apply_delay",
	"restore_command",
	"recovery_target_timeline",
	"recovery_target",
	"recovery_target_lsn",
	"recovery_target_name",
	"recovery_target_time",
	"recovery_target_xid",
	"recovery_target_timeline",
	"recovery_target_action",
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
	return string(pwBytes), nil
}

// walLevel returns the wal_level value to use.
// if there's an user provided wal_level pg parameters and if its value is
// "logical" then returns it, otherwise returns the default ("hot_standby" for
// pg < 9.6 or "replica" for pg >= 9.6).
func (p *PostgresKeeper) walLevel(db *cluster.DB) string {
	var additionalValidWalLevels = []string{
		"logical", // pg >= 10
	}

	maj, min, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just use "hot_standby" that works for all versions
		log.Warnf("failed to get postgres binary version: %v", err)
		return "hot_standby"
	}

	// set default wal_level
	walLevel := "hot_standby"
	if maj == 9 {
		if min >= 6 {
			walLevel = "replica"
		}
	} else if maj >= 10 {
		walLevel = "replica"
	}

	if db.Spec.PGParameters != nil {
		if l, ok := db.Spec.PGParameters["wal_level"]; ok {
			if util.StringInSlice(additionalValidWalLevels, l) {
				walLevel = l
			}
		}
	}

	return walLevel
}

func (p *PostgresKeeper) walKeepSegments(db *cluster.DB) int {
	walKeepSegments := minWalKeepSegments
	if db.Spec.PGParameters != nil {
		if v, ok := db.Spec.PGParameters["wal_keep_segments"]; ok {
			// ignore wrong wal_keep_segments values
			if configuredWalKeepSegments, err := strconv.Atoi(v); err == nil {
				if configuredWalKeepSegments > walKeepSegments {
					walKeepSegments = configuredWalKeepSegments
				}
			}
		}
	}

	return walKeepSegments
}

func (p *PostgresKeeper) mandatoryPGParameters(db *cluster.DB) common.Parameters {
	return common.Parameters{
		"unix_socket_directories": common.PgUnixSocketDirectories,
		"wal_level":               p.walLevel(db),
		"wal_keep_segments":       fmt.Sprintf("%d", p.walKeepSegments(db)),
		"hot_standby":             "on",
	}
}

func (p *PostgresKeeper) getSUConnParams(db, followedDB *cluster.DB) pg.ConnParams {
	cp := pg.ConnParams{
		"user":             p.pgSUUsername,
		"host":             followedDB.Status.ListenAddress,
		"port":             followedDB.Status.Port,
		"application_name": common.StolonName(db.UID),
		"dbname":           "postgres",
		// prefer ssl if available (already the default for postgres libpq but not for golang lib pq)
		"sslmode": "prefer",
	}
	if p.pgSUAuthMethod != "trust" {
		cp.Set("password", p.pgSUPassword)
	}
	return cp
}

func (p *PostgresKeeper) getReplConnParams(db, followedDB *cluster.DB) pg.ConnParams {
	cp := pg.ConnParams{
		"user":             p.pgReplUsername,
		"host":             followedDB.Status.ListenAddress,
		"port":             followedDB.Status.Port,
		"application_name": common.StolonName(db.UID),
		// prefer ssl if available (already the default for postgres libpq but not for golang lib pq)
		"sslmode": "prefer",
	}
	if p.pgReplAuthMethod != "trust" {
		cp.Set("password", p.pgReplPassword)
	}
	return cp
}

func (p *PostgresKeeper) getLocalConnParams() pg.ConnParams {
	cp := pg.ConnParams{
		"user":   p.pgSUUsername,
		"host":   common.PgUnixSocketDirectories,
		"port":   p.pgPort,
		"dbname": "postgres",
		// no sslmode defined since it's not needed and supported over unix sockets
	}
	if p.pgSUAuthMethod != "trust" {
		cp.Set("password", p.pgSUPassword)
	}
	return cp
}

func (p *PostgresKeeper) getLocalReplConnParams() pg.ConnParams {
	cp := pg.ConnParams{
		"user":     p.pgReplUsername,
		"password": p.pgReplPassword,
		"host":     common.PgUnixSocketDirectories,
		"port":     p.pgPort,
		// no sslmode defined since it's not needed and supported over unix sockets
	}
	if p.pgReplAuthMethod != "trust" {
		cp.Set("password", p.pgReplPassword)
	}
	return cp
}

func (p *PostgresKeeper) createPGParameters(db *cluster.DB) common.Parameters {
	parameters := common.Parameters{}

	// Include init parameters if include config is required
	dbls := p.dbLocalStateCopy()
	if db.Spec.IncludeConfig {
		for k, v := range dbls.InitPGParameters {
			parameters[k] = v
		}
	}

	// Copy user defined pg parameters
	for k, v := range db.Spec.PGParameters {
		parameters[k] = v
	}

	// Add/Replace mandatory PGParameters
	for k, v := range p.mandatoryPGParameters(db) {
		parameters[k] = v
	}

	parameters["listen_addresses"] = p.pgListenAddress

	parameters["port"] = p.pgPort
	// TODO(sgotti) max_replication_slots needs to be at least the
	// number of existing replication slots or startup will
	// fail.
	// TODO(sgotti) changing max_replication_slots requires an
	// instance restart.
	parameters["max_replication_slots"] = strconv.FormatUint(uint64(db.Spec.MaxStandbys), 10)
	// Add some more wal senders, since also the keeper will use them
	// TODO(sgotti) changing max_wal_senders requires an instance restart.
	parameters["max_wal_senders"] = strconv.FormatUint(uint64((db.Spec.MaxStandbys*2)+2+db.Spec.AdditionalWalSenders), 10)

	// required by pg_rewind (if data checksum is enabled it's ignored)
	if db.Spec.UsePgrewind {
		parameters["wal_log_hints"] = "on"
	}

	// Setup synchronous replication
	if db.Spec.SynchronousReplication && (len(db.Spec.SynchronousStandbys) > 0 || len(db.Spec.ExternalSynchronousStandbys) > 0) {
		synchronousStandbys := []string{}
		for _, synchronousStandby := range db.Spec.SynchronousStandbys {
			synchronousStandbys = append(synchronousStandbys, common.StolonName(synchronousStandby))
		}
		synchronousStandbys = append(synchronousStandbys, db.Spec.ExternalSynchronousStandbys...)

		// We deliberately don't use postgres FIRST or ANY methods with N
		// different than len(synchronousStandbys) because we need that all the
		// defined standbys are synchronous (so just only one failed standby
		// will block the primary).
		// This is needed for consistency. If we have 3 standbys and we use
		// FIRST 2 (a, b, c), the sentinel, when the master fails, won't be able to know
		// which of the 3 standbys is really synchronous and in sync with the
		// master. And choosing the non synchronous one will cause the loss of
		// the transactions contained in the wal records not transmitted.
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

func (p *PostgresKeeper) createRecoveryOptions(recoveryMode pg.RecoveryMode, standbySettings *cluster.StandbySettings, archiveRecoverySettings *cluster.ArchiveRecoverySettings, recoveryTargetSettings *cluster.RecoveryTargetSettings) *pg.RecoveryOptions {
	parameters := common.Parameters{}

	if standbySettings != nil {
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
		parameters["recovery_target_action"] = "promote"
	}

	return &pg.RecoveryOptions{
		RecoveryMode:       recoveryMode,
		RecoveryParameters: parameters,
	}
}

type PostgresKeeper struct {
	cfg *config

	bootUUID string

	dataDir            string
	pgListenAddress    string
	pgAdvertiseAddress string
	pgPort             string
	pgAdvertisePort    string
	pgBinPath          string
	pgReplAuthMethod   string
	pgReplUsername     string
	pgReplPassword     string
	pgSUAuthMethod     string
	pgSUUsername       string
	pgSUPassword       string

	priority *int // nil means not specified

	sleepInterval  time.Duration
	requestTimeout time.Duration

	e   store.Store
	pgm *pg.Manager
	end chan error

	localStateMutex  sync.Mutex
	keeperLocalState *KeeperLocalState
	dbLocalState     *DBLocalState

	pgStateMutex    sync.Mutex
	getPGStateMutex sync.Mutex
	lastPGState     *cluster.PostgresState

	waitSyncStandbysSynced bool

	canBeMaster             *bool
	canBeSynchronousReplica *bool
}

func NewPostgresKeeper(cfg *config, end chan error) (*PostgresKeeper, error) {
	e, err := cmd.NewStore(&cfg.CommonConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	// Clean and get absolute datadir path
	dataDir, err := filepath.Abs(cfg.dataDir)
	if err != nil {
		return nil, fmt.Errorf("cannot get absolute datadir path for %q: %v", cfg.dataDir, err)
	}

	var priority *int = nil
	if cfg.prioritySpecified {
		priority = &cfg.priority
	}
	p := &PostgresKeeper{
		cfg: cfg,

		bootUUID: common.UUID(),

		dataDir: dataDir,

		pgListenAddress:    cfg.pgListenAddress,
		pgAdvertiseAddress: cfg.pgAdvertiseAddress,
		pgPort:             cfg.pgPort,
		pgAdvertisePort:    cfg.pgAdvertisePort,
		pgBinPath:          cfg.pgBinPath,
		pgReplAuthMethod:   cfg.pgReplAuthMethod,
		pgReplUsername:     cfg.pgReplUsername,
		pgReplPassword:     cfg.pgReplPassword,
		pgSUAuthMethod:     cfg.pgSUAuthMethod,
		pgSUUsername:       cfg.pgSUUsername,
		pgSUPassword:       cfg.pgSUPassword,

		priority: priority,

		sleepInterval:  cluster.DefaultSleepInterval,
		requestTimeout: cluster.DefaultRequestTimeout,

		keeperLocalState: &KeeperLocalState{},
		dbLocalState:     &DBLocalState{},

		canBeMaster:             &cfg.canBeMaster,
		canBeSynchronousReplica: &cfg.canBeSynchronousReplica,

		e:   e,
		end: end,
	}

	err = p.loadKeeperLocalState()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load keeper local state file: %v", err)
	}
	if p.keeperLocalState.UID != "" && p.cfg.uid != "" && p.keeperLocalState.UID != p.cfg.uid {
		log.Fatalf("saved uid %q differs from configuration uid: %q", p.keeperLocalState.UID, cfg.uid)
	}
	if p.keeperLocalState.UID == "" {
		p.keeperLocalState.UID = cfg.uid
		if cfg.uid == "" {
			p.keeperLocalState.UID = common.UID()
			log.Infow("uid generated", "uid", p.keeperLocalState.UID)
		}
		if err = p.saveKeeperLocalState(); err != nil {
			log.Fatalf("error: %v", err)
		}
	}

	log.Infow("keeper uid", "uid", p.keeperLocalState.UID)

	err = p.loadDBLocalState()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load db local state file: %v", err)
	}
	return p, nil
}

func (p *PostgresKeeper) dbLocalStateCopy() *DBLocalState {
	p.localStateMutex.Lock()
	defer p.localStateMutex.Unlock()
	return p.dbLocalState.DeepCopy()
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

	maj, min, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just report maj and min as 0
		log.Warnf("failed to get postgres binary version: %v", err)
	}

	keeperInfo := &cluster.KeeperInfo{
		InfoUID:    common.UID(),
		UID:        keeperUID,
		ClusterUID: clusterUID,
		BootUUID:   p.bootUUID,
		PostgresBinaryVersion: cluster.PostgresBinaryVersion{
			Maj: maj,
			Min: min,
		},
		Priority:      p.priority,
		PostgresState: p.getLastPGState(),

		CanBeMaster:             p.canBeMaster,
		CanBeSynchronousReplica: p.canBeSynchronousReplica,
	}

	// The time to live is just to automatically remove old entries, it's
	// not used to determine if the keeper info has been updated.
	if err := p.e.SetKeeperInfo(context.TODO(), keeperUID, keeperInfo, p.sleepInterval); err != nil {
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

func (p *PostgresKeeper) GetInSyncStandbys() ([]string, error) {
	inSyncStandbysFullName, err := p.pgm.GetSyncStandbys()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve current sync standbys status from instance: %v", err)
	}

	inSyncStandbys := []string{}
	for _, s := range inSyncStandbysFullName {
		if common.IsStolonName(s) {
			inSyncStandbys = append(inSyncStandbys, common.NameFromStolonName(s))
		}
	}

	return inSyncStandbys, nil
}

func (p *PostgresKeeper) GetPGState(pctx context.Context) (*cluster.PostgresState, error) {
	p.getPGStateMutex.Lock()
	defer p.getPGStateMutex.Unlock()
	// Just get one pgstate at a time to avoid exausting available connections
	pgState := &cluster.PostgresState{}

	dbls := p.dbLocalStateCopy()
	pgState.UID = dbls.UID
	pgState.Generation = dbls.Generation

	pgState.ListenAddress = p.pgAdvertiseAddress
	pgState.Port = p.pgAdvertisePort

	initialized, err := p.pgm.IsInitialized()
	if err != nil {
		return pgState, err
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

		inSyncStandbys, err := p.GetInSyncStandbys()
		if err != nil {
			log.Errorw("failed to retrieve current in sync standbys from instance", zap.Error(err))
			return pgState, nil
		}

		pgState.SynchronousStandbys = inSyncStandbys

		sd, err := p.pgm.GetSystemData()
		if err != nil {
			log.Errorw("error getting pg state", zap.Error(err))
			return pgState, nil
		}
		pgState.SystemID = sd.SystemID
		pgState.TimelineID = sd.TimelineID
		pgState.XLogPos = sd.XLogPos

		ctlsh, err := getTimeLinesHistory(pgState, p.pgm, maxPostgresTimelinesHistory)
		if err != nil {
			log.Errorw("error getting timeline history", zap.Error(err))
			return pgState, nil
		}
		pgState.TimelinesHistory = ctlsh

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

func getTimeLinesHistory(pgState *cluster.PostgresState, pgm pg.PGManager, maxPostgresTimelinesHistory int) (cluster.PostgresTimelinesHistory, error) {
	ctlsh := cluster.PostgresTimelinesHistory{}
	// if timeline <= 1 then no timeline history file exists.
	if pgState.TimelineID > 1 {
		var tlsh []*pg.TimelineHistory
		tlsh, err := pgm.GetTimelinesHistory(pgState.TimelineID)
		if err != nil {
			log.Errorw("error getting timeline history", zap.Error(err))
			return ctlsh, err
		}
		if len(tlsh) > maxPostgresTimelinesHistory {
			tlsh = tlsh[len(tlsh)-maxPostgresTimelinesHistory:]
		}
		for _, tlh := range tlsh {
			ctlh := &cluster.PostgresTimelineHistory{
				TimelineID:  tlh.TimelineID,
				SwitchPoint: tlh.SwitchPoint,
				Reason:      tlh.Reason,
			}
			ctlsh = append(ctlsh, ctlh)
		}
	}
	return ctlsh, nil
}

func (p *PostgresKeeper) getLastPGState() *cluster.PostgresState {
	p.pgStateMutex.Lock()
	pgState := p.lastPGState.DeepCopy()
	p.pgStateMutex.Unlock()
	log.Debugf("pgstate dump: %s", spew.Sdump(pgState))
	return pgState
}

func (p *PostgresKeeper) Start(ctx context.Context) {
	endSMCh := make(chan struct{})
	endPgStatecheckerCh := make(chan struct{})
	endUpdateKeeperInfo := make(chan struct{})

	var err error
	var cd *cluster.ClusterData
	cd, _, err = p.e.GetClusterData(context.TODO())
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
	pgm := pg.NewManager(p.pgBinPath, p.dataDir, p.getLocalConnParams(), p.getLocalReplConnParams(), p.pgSUAuthMethod, p.pgSUUsername, p.pgSUPassword, p.pgReplAuthMethod, p.pgReplUsername, p.pgReplPassword, p.requestTimeout)
	p.pgm = pgm

	_ = p.pgm.StopIfStarted(true)

	smTimerCh := time.NewTimer(0).C
	updatePGStateTimerCh := time.NewTimer(0).C
	updateKeeperInfoTimerCh := time.NewTimer(0).C
	for {
		// The sleepInterval can be updated during normal execution. Ensure we regularly
		// refresh the metric to account for those changes.
		sleepInterval.Set(float64(p.sleepInterval / time.Second))

		select {
		case <-ctx.Done():
			log.Debugw("stopping stolon keeper")
			if err = p.pgm.StopIfStarted(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
			}
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

func (p *PostgresKeeper) resync(db, masterDB, followedDB *cluster.DB, tryPgrewind bool) error {
	pgm := p.pgm
	replConnParams := p.getReplConnParams(db, followedDB)
	standbySettings := &cluster.StandbySettings{PrimaryConninfo: replConnParams.ConnString(), PrimarySlotName: common.StolonName(db.UID)}

	// TODO(sgotti) Actually we don't check if pg_rewind is installed or if
	// postgresql version is > 9.5 since someone can also use an externally
	// installed pg_rewind for postgres 9.4. If a pg_rewind executable
	// doesn't exists pgm.SyncFromFollowedPGRewind will return an error and
	// fallback to pg_basebackup
	if tryPgrewind && p.usePgrewind(db) {
		// pg_rewind doesn't support running against a database that is in recovery, as it
		// builds temporary tables and this is not supported on a hot-standby. Stolon doesn't
		// currently support cascading replication, but we should be clear when issuing a
		// rewind that it targets the current primary, rather than whatever database we
		// follow.
		connParams := p.getSUConnParams(db, masterDB)
		log.Infow("syncing using pg_rewind", "masterDB", masterDB.UID, "keeper", followedDB.Spec.KeeperUID)
		if err := pgm.SyncFromFollowedPGRewind(connParams, p.pgSUPassword); err != nil {
			// log pg_rewind error and fallback to pg_basebackup
			log.Errorw("error syncing with pg_rewind", zap.Error(err))
		} else {
			pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))
			return nil
		}
	}

	maj, min, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just don't use replSlot
		log.Warnf("failed to get postgres binary version: %v", err)
	}
	replSlot := ""
	if (maj == 9 && min >= 6) || maj > 10 {
		replSlot = common.StolonName(db.UID)
	}

	if err := pgm.RemoveAll(); err != nil {
		return fmt.Errorf("failed to remove the postgres data dir: %v", err)
	}
	if slog.IsDebug() {
		log.Debugw("syncing from followed db", "followedDB", followedDB.UID, "keeper", followedDB.Spec.KeeperUID, "replConnParams", fmt.Sprintf("%v", replConnParams))
	} else {
		log.Infow("syncing from followed db", "followedDB", followedDB.UID, "keeper", followedDB.Spec.KeeperUID)
	}

	if err := pgm.SyncFromFollowed(replConnParams, replSlot); err != nil {
		return fmt.Errorf("sync error: %v", err)
	}
	log.Infow("sync succeeded")

	pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))

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

func (p *PostgresKeeper) updateReplSlots(curReplSlots []string, uid string, followersUIDs, additionalReplSlots []string) error {
	internalReplSlots := map[string]struct{}{}

	// Create a list of the wanted internal replication slots
	for _, followerUID := range followersUIDs {
		if followerUID == uid {
			continue
		}
		internalReplSlots[common.StolonName(followerUID)] = struct{}{}
	}

	// Add AdditionalReplicationSlots
	for _, slot := range additionalReplSlots {
		internalReplSlots[common.StolonName(slot)] = struct{}{}
	}

	// Drop internal replication slots
	for _, slot := range curReplSlots {
		if !common.IsStolonName(slot) {
			continue
		}
		if _, ok := internalReplSlots[slot]; !ok {
			log.Infow("dropping replication slot", "slot", slot)
			if err := p.pgm.DropReplicationSlot(slot); err != nil {
				log.Errorw("failed to drop replication slot", "slot", slot, "err", err)
				// don't return the error but continue also if drop failed (standby still connected)
			}
		}
	}

	// Create internal replication slots
	for slot := range internalReplSlots {
		if !util.StringInSlice(curReplSlots, slot) {
			log.Infow("creating replication slot", "slot", slot)
			if err := p.pgm.CreateReplicationSlot(slot); err != nil {
				log.Errorw("failed to create replication slot", "slot", slot, zap.Error(err))
				return err
			}
		}
	}
	return nil
}

func (p *PostgresKeeper) refreshReplicationSlots(cd *cluster.ClusterData, db *cluster.DB) error {
	var currentReplicationSlots []string
	currentReplicationSlots, err := p.pgm.GetReplicationSlots()
	if err != nil {
		log.Errorw("failed to get replication slots", zap.Error(err))
		return err
	}

	followersUIDs := db.Spec.Followers

	if err = p.updateReplSlots(currentReplicationSlots, db.UID, followersUIDs, db.Spec.AdditionalReplicationSlots); err != nil {
		log.Errorw("error updating replication slots", zap.Error(err))
		return err
	}

	return nil
}

func (p *PostgresKeeper) postgresKeeperSM(pctx context.Context) {
	e := p.e
	pgm := p.pgm

	cd, _, err := e.GetClusterData(pctx)
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

	// Mark that the clusterdata we've received is valid. We'll use this metric to detect
	// when our store is failing to serve a valid clusterdata, so it's important we only
	// update the metric here.
	clusterdataLastValidUpdateSeconds.SetToCurrentTime()

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
		if err = pgm.StopIfStarted(true); err != nil {
			log.Errorw("failed to stop pg instance", zap.Error(err))
		}
		return
	}

	if p.bootUUID != k.Status.BootUUID {
		log.Infow("our db boot UID is different than the cluster data one, waiting for it to be updated", "bootUUID", p.bootUUID, "clusterBootUUID", k.Status.BootUUID)
		if err = pgm.StopIfStarted(true); err != nil {
			log.Errorw("failed to stop pg instance", zap.Error(err))
		}
		return
	}

	// Generate hba auth from clusterData
	pgm.SetHba(p.generateHBA(cd, db, p.waitSyncStandbysSynced))

	var pgParameters common.Parameters

	dbls := p.dbLocalStateCopy()
	if dbls.Initializing {
		// If we are here this means that the db initialization or
		// resync has failed so we have to clean up stale data
		log.Errorw("db failed to initialize or resync")

		if err = pgm.StopIfStarted(true); err != nil {
			log.Errorw("failed to stop pg instance", zap.Error(err))
			return
		}

		// Clean up cluster db datadir
		if err = pgm.RemoveAll(); err != nil {
			log.Errorw("failed to remove the postgres data dir", zap.Error(err))
			return
		}
		// Reset current db local state since it's not valid anymore
		ndbls := &DBLocalState{
			UID:          "",
			Generation:   cluster.NoGeneration,
			Initializing: false,
		}
		if err = p.saveDBLocalState(ndbls); err != nil {
			log.Errorw("failed to save db local state", zap.Error(err))
			return
		}
	}

	if p.dbLocalState.UID != db.UID {
		var initialized bool
		initialized, err = pgm.IsInitialized()
		if err != nil {
			log.Errorw("failed to detect if instance is initialized", zap.Error(err))
			return
		}
		log.Infow("current db UID different than cluster data db UID", "db", p.dbLocalState.UID, "cdDB", db.UID)

		pgm.SetRecoveryOptions(nil)
		p.waitSyncStandbysSynced = false

		switch db.Spec.InitMode {
		case cluster.DBInitModeNew:
			log.Infow("initializing the database cluster")
			ndbls := &DBLocalState{
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			initConfig := &pg.InitConfig{}

			if db.Spec.NewConfig != nil {
				initConfig.Locale = db.Spec.NewConfig.Locale
				initConfig.Encoding = db.Spec.NewConfig.Encoding
				initConfig.DataChecksums = db.Spec.NewConfig.DataChecksums
			}

			if err = pgm.StopIfStarted(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}
			if err = pgm.RemoveAll(); err != nil {
				log.Errorw("failed to remove the postgres data dir", zap.Error(err))
				return
			}
			if err = pgm.Init(initConfig); err != nil {
				log.Errorw("failed to initialize postgres database cluster", zap.Error(err))
				return
			}

			if err = pgm.StartTmpMerged(); err != nil {
				log.Errorw("failed to start instance", zap.Error(err))
				return
			}
			if err = pgm.WaitReady(cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				log.Errorw("timeout waiting for instance to be ready", zap.Error(err))
				return
			}
			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters()
				if err != nil {
					log.Errorw("failed to retrieve postgres parameters", zap.Error(err))
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					log.Errorw("failed to save db local state", zap.Error(err))
					return
				}
			}

			log.Infow("setting roles")
			if err = pgm.SetupRoles(); err != nil {
				log.Errorw("failed to setup roles", zap.Error(err))
				return
			}

			if err = pgm.StopIfStarted(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}
		case cluster.DBInitModePITR:
			log.Infow("restoring the database cluster")
			ndbls := &DBLocalState{
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if err = pgm.StopIfStarted(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
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

			recoveryMode := pg.RecoveryModeRecovery
			var standbySettings *cluster.StandbySettings
			if db.Spec.FollowConfig != nil && db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
				recoveryMode = pg.RecoveryModeStandby
				standbySettings = db.Spec.FollowConfig.StandbySettings
			}

			pgm.SetRecoveryOptions(p.createRecoveryOptions(recoveryMode, standbySettings, db.Spec.PITRConfig.ArchiveRecoverySettings, db.Spec.PITRConfig.RecoveryTargetSettings))

			if err = pgm.StartTmpMerged(); err != nil {
				log.Errorw("failed to start instance", zap.Error(err))
				return
			}

			if recoveryMode == pg.RecoveryModeRecovery {
				// wait for the db having replyed all the wals
				log.Infof("waiting for recovery to be completed")
				if err = pgm.WaitRecoveryDone(cd.Cluster.DefSpec().SyncTimeout.Duration); err != nil {
					log.Errorw("recovery not finished", zap.Error(err))
					return
				}
				log.Infof("recovery completed")
			}
			if err = pgm.WaitReady(cd.Cluster.DefSpec().SyncTimeout.Duration); err != nil {
				log.Errorw("timeout waiting for instance to be ready", zap.Error(err))
				return
			}

			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters()
				if err != nil {
					log.Errorw("failed to retrieve postgres parameters", zap.Error(err))
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					log.Errorw("failed to save db local state", zap.Error(err))
					return
				}
			}

			if err = pgm.StopIfStarted(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}
		case cluster.DBInitModeResync:
			log.Infow("resyncing the database cluster")
			ndbls := &DBLocalState{
				// replace our current db uid with the required one.
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}

			if err = pgm.StopIfStarted(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}

			// create postgres parameters with empty InitPGParameters
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

			masterDB, ok := cd.DBs[cd.Cluster.Status.Master]
			if tryPgrewind && !ok {
				log.Warn("no current master, disabling pg_rewind for this resync")
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
			if err = p.resync(db, masterDB, followedDB, tryPgrewind); err != nil {
				log.Errorw("failed to resync from followed instance", zap.Error(err))
				return
			}
			if err = pgm.Start(); err != nil {
				log.Errorw("failed to start instance", zap.Error(err))
				return
			}

			if tryPgrewind {
				fullResync := false
				// if not accepting connection assume that it's blocked waiting for missing wal
				// (see above TODO), so do a full resync using pg_basebackup.
				if err = pgm.WaitReady(cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
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
					if err = pgm.StopIfStarted(true); err != nil {
						log.Errorw("failed to stop pg instance", zap.Error(err))
						return
					}
					if err = p.resync(db, masterDB, followedDB, false); err != nil {
						log.Errorw("failed to resync from followed instance", zap.Error(err))
						return
					}
				}
			}

		case cluster.DBInitModeExisting:
			ndbls := &DBLocalState{
				// replace our current db uid with the required one.
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: false,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				log.Errorw("failed to save db local state", zap.Error(err))
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if err = pgm.StopIfStarted(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}
			if err = pgm.StartTmpMerged(); err != nil {
				log.Errorw("failed to start instance", zap.Error(err))
				return
			}
			if err = pgm.WaitReady(cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				log.Errorw("timeout waiting for instance to be ready", zap.Error(err))
				return
			}
			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters()
				if err != nil {
					log.Errorw("failed to retrieve postgres parameters", zap.Error(err))
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					log.Errorw("failed to save db local state", zap.Error(err))
					return
				}
			}
			if err = pgm.StopIfStarted(true); err != nil {
				log.Errorw("failed to stop pg instance", zap.Error(err))
				return
			}
		case cluster.DBInitModeNone:
			log.Errorw("different local dbUID but init mode is none, this shouldn't happen. Something bad happened to the keeper data. Check that keeper data is on a persistent volume and that the keeper state files weren't removed")
			return
		default:
			log.Errorw("unknown db init mode", "initMode", string(db.Spec.InitMode))
			return
		}
	}

	initialized, err := pgm.IsInitialized()
	if err != nil {
		log.Errorw("failed to detect if instance is initialized", zap.Error(err))
		return
	}

	if initialized {
		var started bool
		started, err = pgm.IsStarted()
		if err != nil {
			// log error getting instance state but go ahead.
			log.Errorw("failed to retrieve instance status", zap.Error(err))
		}
		log.Debugw("db status", "initialized", true, "started", started)
	} else {
		log.Debugw("db status", "initialized", false, "started", false)
	}

	// create postgres parameters
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

	// Set metrics to power alerts about mismatched roles
	setRole(localRoleGauge, &localRole)
	setRole(targetRoleGauge, &targetRole)

	switch targetRole {
	case common.RoleMaster:
		// We are the elected master
		log.Infow("our db requested role is master")
		if localRole == common.RoleUndefined {
			log.Errorw("database cluster not initialized but requested role is master. This shouldn't happen!")
			return
		}
		started, err := pgm.IsStarted()
		if err != nil {
			log.Errorw("failed to retrieve instance status", zap.Error(err))
			return
		}
		if !started {
			// if we have syncrepl enabled and the postgres instance is stopped, before opening connections to normal users wait for having the defined synchronousStandbys in sync state.
			if db.Spec.SynchronousReplication {
				p.waitSyncStandbysSynced = true
				log.Infow("not allowing connection as normal users since synchronous replication is enabled and instance was down")
				pgm.SetHba(p.generateHBA(cd, db, true))
			}

			if err = pgm.Start(); err != nil {
				log.Errorw("failed to start postgres", zap.Error(err))
				return
			}
			if err = pgm.WaitReady(cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				log.Errorw("timeout waiting for instance to be ready", zap.Error(err))
				return
			}
		}

		if localRole == common.RoleStandby {
			log.Infow("promoting to master")
			pgm.SetRecoveryOptions(nil)
			if err = pgm.Promote(); err != nil {
				log.Errorw("failed to promote instance", zap.Error(err))
				return
			}
		} else {
			log.Infow("already master")
		}

		if err := p.refreshReplicationSlots(cd, db); err != nil {
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
			started, err := pgm.IsStarted()
			if err != nil {
				log.Errorw("failed to retrieve instance status", zap.Error(err))
				return
			}
			if !started {
				pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))
				if err = pgm.Start(); err != nil {
					log.Errorw("failed to start postgres", zap.Error(err))
					return
				}
			}

			// Update our primary_conninfo if replConnString changed
			switch db.Spec.FollowConfig.Type {
			case cluster.FollowTypeInternal:
				followedUID := db.Spec.FollowConfig.DBUID
				followedDB, ok := cd.DBs[followedUID]
				if !ok {
					log.Errorw("no db data available for followed db", "followedDB", followedUID)
					return
				}
				newReplConnParams := p.getReplConnParams(db, followedDB)
				log.Debugw("newReplConnParams", "newReplConnParams", newReplConnParams)

				standbySettings := &cluster.StandbySettings{PrimaryConninfo: newReplConnParams.ConnString(), PrimarySlotName: common.StolonName(db.UID)}

				curRecoveryOptions := pgm.CurRecoveryOptions()
				newRecoveryOptions := p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil)

				// Update recovery conf if parameters has changed
				if !curRecoveryOptions.RecoveryParameters.Equals(newRecoveryOptions.RecoveryParameters) {
					log.Infow("recovery parameters changed, restarting postgres instance", "curRecoveryParameters", curRecoveryOptions.RecoveryParameters, "newRecoveryParameters", newRecoveryOptions.RecoveryParameters)
					pgm.SetRecoveryOptions(newRecoveryOptions)

					if err = pgm.Restart(true); err != nil {
						log.Errorw("failed to restart postgres instance", zap.Error(err))
						return
					}
				}

				if err = p.refreshReplicationSlots(cd, db); err != nil {
					log.Errorw("error updating replication slots", zap.Error(err))
				}

			case cluster.FollowTypeExternal:
				curRecoveryOptions := pgm.CurRecoveryOptions()
				newRecoveryOptions := p.createRecoveryOptions(pg.RecoveryModeStandby, db.Spec.FollowConfig.StandbySettings, db.Spec.FollowConfig.ArchiveRecoverySettings, nil)

				// Update recovery conf if parameters has changed
				if !curRecoveryOptions.RecoveryParameters.Equals(newRecoveryOptions.RecoveryParameters) {
					log.Infow("recovery parameters changed, restarting postgres instance", "curRecoveryParameters", curRecoveryOptions.RecoveryParameters, "newRecoveryParameters", newRecoveryOptions.RecoveryParameters)
					pgm.SetRecoveryOptions(newRecoveryOptions)

					if err = pgm.Restart(true); err != nil {
						log.Errorw("failed to restart postgres instance", zap.Error(err))
						return
					}
				}

				if err = p.refreshReplicationSlots(cd, db); err != nil {
					log.Errorw("error updating replication slots", zap.Error(err))
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
	prevSyncStandbyNames := pgm.CurParameters()["synchronous_standby_names"]
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

	needsReload := false
	changedParams := pgParameters.Diff(pgm.CurParameters())

	if !pgParameters.Equals(pgm.CurParameters()) {
		log.Infow("postgres parameters changed, reloading postgres instance")
		pgm.SetParameters(pgParameters)
		needsReload = true
	} else {
		// for tests
		log.Infow("postgres parameters not changed")
	}

	// Generate hba auth from clusterData

	// if we have syncrepl enabled and the postgres instance is stopped, before opening connections to normal users wait for having the defined synchronousStandbys in sync state.
	if db.Spec.SynchronousReplication && p.waitSyncStandbysSynced {
		inSyncStandbys, err := p.GetInSyncStandbys()
		if err != nil {
			log.Errorw("failed to retrieve current in sync standbys from instance", zap.Error(err))
			return
		}
		if !util.CompareStringSliceNoOrder(inSyncStandbys, db.Spec.SynchronousStandbys) {
			log.Infow("not allowing connection as normal users since synchronous replication is enabled, instance was down and not all sync standbys are synced")
		} else {
			p.waitSyncStandbysSynced = false
		}
	} else {
		p.waitSyncStandbysSynced = false
	}
	newHBA := p.generateHBA(cd, db, p.waitSyncStandbysSynced)
	if !reflect.DeepEqual(newHBA, pgm.CurHba()) {
		log.Infow("postgres hba entries changed, reloading postgres instance")
		pgm.SetHba(newHBA)
		needsReload = true
	} else {
		// for tests
		log.Infow("postgres hba entries not changed")
	}

	if needsReload {
		needsReloadGauge.Set(1) // mark as reload needed
		if err := pgm.Reload(); err != nil {
			log.Errorw("failed to reload postgres instance", err)
		} else {
			needsReloadGauge.Set(0) // successful reload implies no longer required
		}
	}

	{
		clusterSpec := cd.Cluster.DefSpec()
		automaticPgRestartEnabled := *clusterSpec.AutomaticPgRestart

		needsRestart, err := pgm.IsRestartRequired(changedParams)
		if err != nil {
			log.Errorw("failed to check if restart is required", zap.Error(err))
		}

		if needsRestart {
			needsRestartGauge.Set(1) // mark as restart needed
			if automaticPgRestartEnabled {
				log.Infow("restarting postgres")
				if err := pgm.Restart(true); err != nil {
					log.Errorw("failed to restart postgres instance", zap.Error(err))
				} else {
					needsRestartGauge.Set(0) // successful restart implies no longer required
				}
			}
		}
	}

	// If we are here, then all went well and we can update the db generation and save it locally
	ndbls := p.dbLocalStateCopy()
	ndbls.Generation = db.Generation
	ndbls.Initializing = false
	if err := p.saveDBLocalState(ndbls); err != nil {
		log.Errorw("failed to save db local state", zap.Error(err))
		return
	}

	// We want to set this only if no error has occurred. We should be able to identify
	// keeper issues by watching for this value becoming stale.
	lastSyncSuccessSeconds.SetToCurrentTime()
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
	return common.WriteFileAtomic(p.keeperLocalStateFilePath(), 0600, sj)
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

// saveDBLocalState saves on disk the dbLocalState and only if successful
// updates the current in memory state
func (p *PostgresKeeper) saveDBLocalState(dbls *DBLocalState) error {
	sj, err := json.Marshal(dbls)
	if err != nil {
		return err
	}
	if err = common.WriteFileAtomic(p.dbLocalStateFilePath(), 0600, sj); err != nil {
		return err
	}

	p.localStateMutex.Lock()
	p.dbLocalState = dbls.DeepCopy()
	p.localStateMutex.Unlock()

	return nil
}

// IsMaster return if the db is the cluster master db.
// A master is a db that:
// * Has a master db role
// or
// * Has a standby db role with followtype external
func IsMaster(db *cluster.DB) bool {
	switch db.Spec.Role {
	case common.RoleMaster:
		return true
	case common.RoleStandby:
		if db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
			return true
		}
		return false
	default:
		panic("invalid db role in db Spec")
	}
}

// generateHBA generates the instance hba entries depending on the value of
// DefaultSUReplAccessMode.
// When onlyInternal is true only rules needed for replication will be setup
// and the traffic should be permitted only for pgSUUsername standard
// connections and pgReplUsername replication connections.
func (p *PostgresKeeper) generateHBA(cd *cluster.ClusterData, db *cluster.DB, onlyInternal bool) []string {
	// Minimal entries for local normal and replication connections needed by the stolon keeper
	// Matched local connections are for postgres database and suUsername user with md5 auth
	// Matched local replication connections are for replUsername user with md5 auth
	computedHBA := []string{
		fmt.Sprintf("local postgres %s %s", p.pgSUUsername, p.pgSUAuthMethod),
		fmt.Sprintf("local replication %s %s", p.pgReplUsername, p.pgReplAuthMethod),
	}

	switch *cd.Cluster.DefSpec().DefaultSUReplAccessMode {
	case cluster.SUReplAccessAll:
		// all the keepers will accept connections from every host
		computedHBA = append(
			computedHBA,
			fmt.Sprintf("host all %s %s %s", p.pgSUUsername, "0.0.0.0/0", p.pgSUAuthMethod),
			fmt.Sprintf("host all %s %s %s", p.pgSUUsername, "::0/0", p.pgSUAuthMethod),
			fmt.Sprintf("host replication %s %s %s", p.pgReplUsername, "0.0.0.0/0", p.pgReplAuthMethod),
			fmt.Sprintf("host replication %s %s %s", p.pgReplUsername, "::0/0", p.pgReplAuthMethod),
		)
	case cluster.SUReplAccessStrict:
		// only the master keeper (primary instance or standby of a remote primary when in standby cluster mode) will accept connections only from the other standby keepers IPs
		if IsMaster(db) {
			addresses := []string{}
			for _, dbElt := range cd.DBs {
				if dbElt.UID != db.UID {
					addresses = append(addresses, dbElt.Status.ListenAddress)
				}
			}
			sort.Strings(addresses)
			for _, address := range addresses {
				computedHBA = append(
					computedHBA,
					fmt.Sprintf("host all %s %s/32 %s", p.pgSUUsername, address, p.pgReplAuthMethod),
					fmt.Sprintf("host replication %s %s/32 %s", p.pgReplUsername, address, p.pgReplAuthMethod),
				)
			}
		}
	}

	if !onlyInternal {
		// By default, if no custom pg_hba entries are provided, accept
		// connections for all databases and users with md5 auth
		if db.Spec.PGHBA != nil {
			computedHBA = append(computedHBA, db.Spec.PGHBA...)
		} else {
			computedHBA = append(
				computedHBA,
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			)
		}
	}

	// return generated Hba merged with user Hba
	return computedHBA
}

func sigHandler(sigs chan os.Signal, cancel context.CancelFunc) {
	s := <-sigs
	log.Debugw("got signal", "signal", s)
	shutdownSeconds.SetToCurrentTime()
	cancel()
}

func Execute() {
	if err := flagutil.SetFlagsFromEnv(CmdKeeper.PersistentFlags(), "STKEEPER"); err != nil {
		log.Fatal(err)
	}

	if err := CmdKeeper.Execute(); err != nil {
		log.Fatal(err)
	}
}

func keeper(c *cobra.Command, args []string) {
	var (
		err           error
		listenAddFlag = "pg-advertise-address"
	)

	flags := c.Flags()

	if !flags.Changed("pg-su-username") {
		// set the pgSuUsername to the current user
		var user string
		user, err = util.GetUser()
		if err != nil {
			log.Fatalf("cannot get current user: %v", err)
		}
		cfg.pgSUUsername = user
	}

	validAuthMethods := make(map[string]struct{})
	validAuthMethods["trust"] = struct{}{}
	validAuthMethods["md5"] = struct{}{}
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
		log.Fatalf("invalid log level: %v", cfg.LogLevel)
	}
	if cfg.debug {
		slog.SetDebug()
	}
	if cmd.IsColorLoggerEnable(c, &cfg.CommonConfig) {
		log = slog.SColor()
		pg.SetLogger(log)
	}

	if cfg.dataDir == "" {
		log.Fatalf("data dir required")
	}

	if err = cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		log.Fatalf(err.Error())
	}

	cmd.SetMetrics(&cfg.CommonConfig, "keeper")

	if err = os.MkdirAll(cfg.dataDir, 0700); err != nil {
		log.Fatalf("cannot create data dir: %v", err)
	}

	if cfg.pgListenAddress == "" {
		log.Fatalf("--pg-listen-address is required")
	}

	if cfg.pgAdvertiseAddress == "" {
		listenAddFlag = "pg-listen-address"
		cfg.pgAdvertiseAddress = cfg.pgListenAddress
	}

	if cfg.pgAdvertisePort == "" {
		cfg.pgAdvertisePort = cfg.pgPort
	}

	ip := net.ParseIP(cfg.pgAdvertiseAddress)
	if ip == nil {
		log.Warnf("provided --%s %q: is not an ip address but a hostname. This will be advertized to the other components and may have undefined behaviors if resolved differently by other hosts", listenAddFlag, cfg.pgAdvertiseAddress)
	}

	ipAddr, err := net.ResolveIPAddr("ip", cfg.pgAdvertiseAddress)
	if err != nil {
		log.Warnf("cannot resolve provided --%s %q: %v", listenAddFlag, cfg.pgAdvertiseAddress, err)
	} else {
		if ipAddr.IP.IsLoopback() {
			log.Warnf("provided --%s %q is a loopback ip. This will be advertized to the other components and communication will fail if they are on different hosts", listenAddFlag, cfg.pgAdvertiseAddress)
		}
	}

	if _, ok := validAuthMethods[cfg.pgReplAuthMethod]; !ok {
		log.Fatalf("--pg-repl-auth-method must be one of: md5, trust")
	}
	if cfg.pgReplUsername == "" {
		log.Fatalf("--pg-repl-username is required")
	}
	if cfg.pgReplAuthMethod == "trust" {
		log.Warn("not utilizing a password for replication between hosts is extremely dangerous")
		if cfg.pgReplPassword != "" || cfg.pgReplPasswordFile != "" {
			log.Fatalf("can not utilize --pg-repl-auth-method trust together with --pg-repl-password or --pg-repl-passwordfile")
		}
	}
	if cfg.pgSUAuthMethod == "trust" {
		log.Warn("not utilizing a password for superuser is extremely dangerous")
		if cfg.pgSUPassword != "" || cfg.pgSUPasswordFile != "" {
			log.Fatalf("can not utilize --pg-su-auth-method trust together with --pg-su-password or --pg-su-passwordfile")
		}
	}
	if cfg.pgReplAuthMethod != "trust" && cfg.pgReplPassword == "" && cfg.pgReplPasswordFile == "" {
		log.Fatalf("one of --pg-repl-password or --pg-repl-passwordfile is required")
	}
	if cfg.pgReplAuthMethod != "trust" && cfg.pgReplPassword != "" && cfg.pgReplPasswordFile != "" {
		log.Fatalf("only one of --pg-repl-password or --pg-repl-passwordfile must be provided")
	}
	if _, ok := validAuthMethods[cfg.pgSUAuthMethod]; !ok {
		log.Fatalf("--pg-su-auth-method must be one of: md5, password, trust")
	}
	if cfg.pgSUAuthMethod != "trust" && cfg.pgSUPassword == "" && cfg.pgSUPasswordFile == "" {
		log.Fatalf("one of --pg-su-password or --pg-su-passwordfile is required")
	}
	if cfg.pgSUAuthMethod != "trust" && cfg.pgSUPassword != "" && cfg.pgSUPasswordFile != "" {
		log.Fatalf("only one of --pg-su-password or --pg-su-passwordfile must be provided")
	}

	if cfg.pgReplPasswordFile != "" {
		cfg.pgReplPassword, err = readPasswordFromFile(cfg.pgReplPasswordFile)
		if err != nil {
			log.Fatalf("cannot read pg replication user password: %v", err)
		}
	}
	if cfg.pgSUPasswordFile != "" {
		cfg.pgSUPassword, err = readPasswordFromFile(cfg.pgSUPasswordFile)
		if err != nil {
			log.Fatalf("cannot read pg superuser password: %v", err)
		}
	}

	// Trim trailing new lines from passwords
	tp := strings.TrimRight(cfg.pgSUPassword, "\r\n")
	if cfg.pgSUPassword != tp {
		log.Warn("superuser password contain trailing new line, removing")
		if tp == "" {
			log.Fatalf("superuser password is empty after removing trailing new line")
		}
		cfg.pgSUPassword = tp
	}

	tp = strings.TrimRight(cfg.pgReplPassword, "\r\n")
	if cfg.pgReplPassword != tp {
		log.Warn("replication user password contain trailing new line, removing")
		if tp == "" {
			log.Fatalf("replication user password is empty after removing trailing new line")
		}
		cfg.pgReplPassword = tp
	}

	if cfg.pgSUUsername == cfg.pgReplUsername {
		log.Warn("superuser name and replication user name are the same. Different users are suggested.")
		if cfg.pgReplAuthMethod != cfg.pgSUAuthMethod {
			log.Fatalf("do not support different auth methods when utilizing superuser for replication.")
		}
		if cfg.pgSUPassword != cfg.pgReplPassword && cfg.pgSUAuthMethod != "trust" && cfg.pgReplAuthMethod != "trust" {
			log.Fatalf("provided superuser name and replication user name are the same but provided passwords are different")
		}
	}

	// if --priority wasn't specified explictily, last value is reused, so
	// remember it
	cfg.prioritySpecified = c.Flags().Changed("priority")

	// Open (and create if needed) the lock file.
	// There is no need to clean up this file since we don't use the file as an actual lock. We get a lock
	// on the file. So the lock get released when our process stops (or log.Fatalfs).
	lockFileName := filepath.Join(cfg.dataDir, "lock")
	lockFile, err := os.OpenFile(lockFileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalf("cannot take exclusive lock on data dir %q: %v", lockFileName, err)
	}

	// Get a lock on our lock file.
	ft := &syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: int16(io.SeekStart),
		Start:  0,
		Len:    0, // Entire file.
	}

	err = syscall.FcntlFlock(lockFile.Fd(), syscall.F_SETLK, ft)
	if err != nil {
		log.Fatalf("cannot take exclusive lock on data dir %q: %v", lockFileName, err)
	}

	log.Infow("exclusive lock on data dir taken")

	if cfg.uid != "" {
		if !pg.IsValidReplSlotName(cfg.uid) {
			log.Fatalf("keeper uid %q not valid. It can contain only lower-case letters, numbers and the underscore character", cfg.uid)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	end := make(chan error)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go sigHandler(sigs, cancel)

	if cfg.MetricsListenAddress != "" {
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			err = http.ListenAndServe(cfg.MetricsListenAddress, nil)
			if err != nil {
				log.Errorw("metrics http server error", zap.Error(err))
				cancel()
			}
		}()
	}

	p, err := NewPostgresKeeper(&cfg, end)
	if err != nil {
		log.Fatalf("cannot create keeper: %v", err)
	}
	go p.Start(ctx)

	<-end

	lockFile.Close()
}
