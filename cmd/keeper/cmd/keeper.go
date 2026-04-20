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

// Package cmd holds all CLI code for the keeper
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	cluster "github.com/pgvillage-tools/stolon/api/v1"
	"github.com/pgvillage-tools/stolon/cmd"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/flagutil"
	"github.com/pgvillage-tools/stolon/internal/logging"
	pg "github.com/pgvillage-tools/stolon/internal/postgresql"
	"github.com/pgvillage-tools/stolon/internal/store"
	"github.com/pgvillage-tools/stolon/internal/util"
	"github.com/rs/zerolog"

	"github.com/davecgh/go-spew/spew"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

const (
	errorMsgPgInst          = "failed to stop pg instance"
	errorMsgDbState         = "failed to save db local state"
	followedStr             = "followedDB"
	ownerRWPermisions       = 0600
	ownerRWExecPermisions   = 0700
	ownerRWOtherRPermisions = 0644
	decimal                 = 10

	authTrust = "trust"
	authMd5   = "md5"
	authCert  = "cert"
	authIdent = "ident"
	authPeer  = "peer"

	connParamUser     = "user"
	connParamPassword = "password"
	connParamHost     = "host"
	connParamPort     = "port"
	connParamAppName  = "application_name"
	connParamDbName   = "dbname"
	connParamSslMode  = "sslmode"

	connTypeHost         = "host"
	connTypeHostSsl      = "hostssl"
	connTypeHostNoSsl    = "hostnossl"
	connTypeHostGssEnc   = "hostgssenc"
	connTypeHostNoGssEnc = "hostnogssenc"

	defaultDatabase = "postgres"
)

// CmdKeeper exports the main keeper process
var CmdKeeper = &cobra.Command{
	Use:     "stolon-keeper",
	Run:     keeper,
	Version: cmd.Version,
}

const (
	maxPostgresTimelinesHistory = 2
	minWalKeepSegments          = 8
)

// KeeperLocalState can be used to define the local state for a keep process
type KeeperLocalState struct {
	UID        string
	ClusterUID string
}

// DBLocalState can be used to store the local state for DB settings
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

// DeepCopy is a function that copies the state of a local database
func (s *DBLocalState) DeepCopy() (dc *DBLocalState) {
	var ok bool
	if s == nil {
		return nil
	}
	if ns, err := copystructure.Copy(s); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(s, ns) {
		panic("not equal")
	} else if dc, ok = ns.(*DBLocalState); !ok {
		panic("different type after copy")
	}
	return dc
}

type config struct {
	cmd.CommonConfig

	uid                   string
	dataDir               string
	walDir                string
	debug                 bool
	pgListenAddress       string
	pgAdvertiseAddress    string
	pgPort                string
	pgAdvertisePort       string
	pgBinPath             string
	pgReplConnType        string
	pgReplAuthMethod      string
	pgReplLocalAuthMethod string
	pgReplSslMode         string
	pgReplUsername        string
	pgReplPassword        string
	pgReplPasswordFile    string
	pgSUConnType          string
	pgSUAuthMethod        string
	pgSULocalAuthMethod   string
	pgSUUsername          string
	pgSUPassword          string
	pgSUPasswordFile      string

	canBeMaster             bool
	canBeSynchronousReplica bool
	disableDataDirLocking   bool
}

var cfg config

func init() {
	ctx := context.Background()
	cmd.AddCommonFlags(CmdKeeper, &cfg.CommonConfig)
	// revive:disable
	CmdKeeper.PersistentFlags().StringVar(&cfg.uid, "id", "", "keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.uid, "uid", "", "keeper uid (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random uid will be generated.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.dataDir, "data-dir", "", "data directory")
	CmdKeeper.PersistentFlags().StringVar(&cfg.walDir, "wal-dir", "", "wal directory")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgListenAddress, "pg-listen-address", "", "postgresql instance listening address, local address used for the postgres instance. For all network interface, you can set the value to '*'.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgAdvertiseAddress, "pg-advertise-address", "", "postgresql instance address from outside. Use it to expose ip different than local ip with a NAT networking config")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgPort, "pg-port", "5432", "postgresql instance listening port")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgAdvertisePort, "pg-advertise-port", "", "postgresql instance port from outside. Use it to expose port different than local port with a PAT networking config")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgBinPath, "pg-bin-path", "", "absolute path to postgresql binaries. If empty they will be searched in the current PATH")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplConnType, "pg-repl-connection-type", connTypeHost, "postgres replication user connection type. Default is host.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplAuthMethod, "pg-repl-auth-method", authMd5, "postgres replication user auth method. Default is authMd5.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplLocalAuthMethod, "pg-repl-local-auth-method", "", "postgres replication user auth method. Default is same as pg-repl-auth-method.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplSslMode, "pg-repl-ssl-mode", "prefer", "postgres replication user ssl-mode. Default is prefer.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplUsername, "pg-repl-username", "", "postgres replication user name. Required. It'll be created on db initialization. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplPassword, "pg-repl-password", "", "postgres replication user password. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgReplPasswordFile, "pg-repl-passwordfile", "", "postgres replication user password file. Only one of --pg-repl-password or --pg-repl-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUConnType, "pg-su-connection-type", connTypeHost, "postgres superuser connection type. Default is host.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUAuthMethod, "pg-su-auth-method", authMd5, "postgres superuser auth method. Default is authMd5.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSULocalAuthMethod, "pg-su-local-auth-method", "", "postgres superuser auth method. Default is same as pg-su-auth-method.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUUsername, "pg-su-username", "", "postgres superuser user name. Used for keeper managed instance access and pg_rewind based synchronization. It'll be created on db initialization. Defaults to the name of the effective user running stolon-keeper. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUPassword, "pg-su-password", "", "postgres superuser password. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers.")
	CmdKeeper.PersistentFlags().StringVar(&cfg.pgSUPasswordFile, "pg-su-passwordfile", "", "postgres superuser password file. Only one of --pg-su-password or --pg-su-passwordfile must be provided. Must be the same for all keepers)")
	CmdKeeper.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")

	CmdKeeper.PersistentFlags().BoolVar(&cfg.canBeMaster, "can-be-master", true, "prevent keeper from being elected as master")
	CmdKeeper.PersistentFlags().BoolVar(&cfg.canBeSynchronousReplica, "can-be-synchronous-replica", true, "prevent keeper from being chosen as synchronous replica")
	CmdKeeper.PersistentFlags().BoolVar(&cfg.disableDataDirLocking, "disable-data-dir-locking", false, "disable locking on data dir. Warning! It'll cause data corruptions if two keepers are concurrently running with the same data dir.")
	// revive:enable

	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	if err := CmdKeeper.PersistentFlags().MarkDeprecated("id", "please use --uid"); err != nil {
		logger.Fatal().AnErr("err", err).Msg("--id is deprecated, please use --uid")
	}
	if err := CmdKeeper.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead"); err != nil {
		logger.Fatal().AnErr("err", err).Msg("--debug is deprecated, please use --log-level=debug instead")
	}
}

var managedPGParameters = []string{
	"unix_socket_directories",
	"wal_keep_segments",
	"wal_keep_size",
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

func readPasswordFromFile(ctx context.Context, filePath string) (string, error) {
	fi, err := os.Lstat(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read password from file %s: %v", filePath, err)
	}
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)

	if fi.Mode() > ownerRWPermisions {
		// TODO: enforce this by exiting with an error. Kubernetes makes this file too open today.
		logger.Warn().Str("file", filePath).Str("mode", fmt.Sprintf("%#o", fi.Mode())).
			Msgf("password file permissions are too open. " +
				"This file should only be readable to the user executing stolon! Continuing...")
	}

	pwBytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read password from file %s: %v", filePath, err)
	}
	return string(pwBytes), nil
}

// walLevel returns the wal_level value to use.
// if there's an user provided wal_level pg parameters and if its value is
// "logical" then returns it, otherwise returns the default ("hot_standby" for
// pg < 9.6 or "replica" for pg >= 9.6).
func (p *PostgresKeeper) walLevel(ctx context.Context, db *cluster.DB) string {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	var additionalValidWalLevels = []string{
		"logical", // pg >= 10
	}

	version, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just use "hot_standby"
		// that works for all versions
		logger.Warn().AnErr("err", err).Msg("failed to get postgres binary version")
		return "hot_standby"
	}

	// set default wal_level
	walLevel := "hot_standby"
	if version.GreaterThanEqual(pg.V96) {
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

func (p *PostgresKeeper) walKeepSize(db *cluster.DB) string {
	// assume default 16Mib wal segment size
	walKeepSize := strconv.Itoa(minWalKeepSegments * 16)

	// TODO(sgotti) currently we ignore if wal_keep_size value is less than our
	// min value or wrong and just return it as is
	if db.Spec.PGParameters != nil {
		if v, ok := db.Spec.PGParameters["wal_keep_size"]; ok {
			return v
		}
	}

	return walKeepSize
}

func (p *PostgresKeeper) mandatoryPGParameters(ctx context.Context, db *cluster.DB) common.Parameters {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	params := common.Parameters{
		"unix_socket_directories": common.PgUnixSocketDirectories,
		"wal_level":               p.walLevel(ctx, db),
		"hot_standby":             "on",
	}

	version, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version don't return any wal_keep_segments or wal_keep_size
		logger.Warn().AnErr("err", err).Msg("failed to get postgres binary version")
		return params
	}

	if version.GreaterThanEqual(pg.V13) {
		params["wal_keep_size"] = p.walKeepSize(db)
	} else {
		params["wal_keep_segments"] = fmt.Sprintf("%d", p.walKeepSegments(db))
	}

	return params
}

func (p *PostgresKeeper) getSUConnParams(db, followedDB *cluster.DB) pg.ConnParams {
	cp := pg.ConnParams{}.
		WithUser(p.pgSUUsername).
		WithHost(followedDB.Status.ListenAddress).
		WithSPort(followedDB.Status.Port).
		WithAppName(common.StolonName(db.UID)).
		WithDbName(defaultDatabase).
		// This is currently only used for pgRewind, which requires a SU (repluser might not be enough).
		// Pgrewind is the only feature using SU over remote connection
		// and with that the only type using SU with sslmode.
		// Therefore we have skipped extra config option for sslmode for SU,
		// and reuse config for sslmode for repl user instead.
		WithSSLMode(p.pgReplSslMode)

	if p.pgSUAuthMethod == authMd5 {
		cp = cp.WithPassword(p.pgSUPassword)
	}
	return cp
}

func (p *PostgresKeeper) getReplConnParams(db, followedDB *cluster.DB) pg.ConnParams {
	cp := pg.ConnParams{}.
		WithUser(p.pgReplUsername).
		WithHost(followedDB.Status.ListenAddress).
		WithSPort(followedDB.Status.Port).
		WithAppName(common.StolonName(db.UID)).
		WithSSLMode("prefer")
	if p.pgReplAuthMethod == authMd5 {
		cp = cp.WithPassword(p.pgReplPassword)
	}
	return cp
}

func (p *PostgresKeeper) getLocalConnParams() pg.ConnParams {
	cp := pg.ConnParams{}.
		WithUser(p.pgSUUsername).
		WithHost(common.PgUnixSocketDirectories).
		WithSPort(p.pgPort).
		WithDbName(defaultDatabase)
		// no sslmode defined since it's not needed and supported over unix sockets
	if p.pgSUAuthMethod == authMd5 {
		cp = cp.WithPassword(p.pgSUPassword)
	}
	return cp
}

func (p *PostgresKeeper) getLocalReplConnParams() pg.ConnParams {
	cp := pg.ConnParams{}.
		WithUser(p.pgReplUsername).
		WithHost(common.PgUnixSocketDirectories).
		WithSPort(p.pgPort)
		// no sslmode defined since it's not needed and supported over unix sockets

	if p.pgReplAuthMethod == authMd5 {
		cp = cp.WithPassword(p.pgReplPassword)
	}
	return cp
}

func (p *PostgresKeeper) createPGParameters(ctx context.Context, db *cluster.DB) common.Parameters {
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
	for k, v := range p.mandatoryPGParameters(ctx, db) {
		parameters[k] = v
	}

	parameters["listen_addresses"] = p.pgListenAddress

	parameters["port"] = p.pgPort
	// TODO(sgotti) max_replication_slots needs to be at least the
	// number of existing replication slots or startup will
	// fail.
	// TODO(sgotti) changing max_replication_slots requires an
	// instance restart.
	parameters["max_replication_slots"] = strconv.FormatUint(uint64(db.Spec.MaxStandbys), decimal)
	// Add some more wal senders, since also the keeper will use them
	// TODO(sgotti) changing max_wal_senders requires an instance restart.
	parameters["max_wal_senders"] = strconv.FormatUint(uint64((db.Spec.MaxStandbys*2)+2+db.Spec.AdditionalWalSenders),
		decimal)

	// required by pg_rewind (if data checksum is enabled it's ignored)
	if db.Spec.UsePgrewind {
		parameters["wal_log_hints"] = "on"
	}

	// Setup synchronous replication
	if db.Spec.SynchronousReplication &&
		(len(db.Spec.SynchronousStandbys) > 0 ||
			len(db.Spec.ExternalSynchronousStandbys) > 0) {
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
			parameters["synchronous_standby_names"] = fmt.Sprintf(
				"%d (%s)",
				len(synchronousStandbys),
				strings.Join(synchronousStandbys,
					","))
		} else {
			parameters["synchronous_standby_names"] = strings.Join(synchronousStandbys, ",")
		}
	} else {
		parameters["synchronous_standby_names"] = ""
	}

	return parameters
}

func (p *PostgresKeeper) createRecoveryOptions(
	recoveryMode pg.RecoveryMode,
	standbySettings *cluster.StandbySettings,
	archiveRecoverySettings *cluster.ArchiveRecoverySettings,
	recoveryTargetSettings *cluster.RecoveryTargetSettings) *pg.RecoveryOptions {
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

// PostgresKeeper is a struct containing information about the postgres server like
// directories, addresses and binpath etc.
type PostgresKeeper struct {
	cfg *config

	bootUUID string

	dataDir               string
	walDir                string
	pgListenAddress       string
	pgAdvertiseAddress    string
	pgPort                string
	pgAdvertisePort       string
	pgBinPath             string
	pgReplConnType        string
	pgReplAuthMethod      string
	pgReplLocalAuthMethod string
	pgReplSslMode         string
	pgReplUsername        string
	pgReplPassword        string
	pgSUConnType          string
	pgSUAuthMethod        string
	pgSULocalAuthMethod   string
	pgSUUsername          string
	pgSUPassword          string

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

// NewPostgresKeeper is function which makes a new postgreskeeper
func NewPostgresKeeper(ctx context.Context, cfg *config, end chan error) (*PostgresKeeper, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ctx, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	e, err := cmd.NewStore(ctx, &cfg.CommonConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	// Clean and get absolute datadir path
	dataDir, err := filepath.Abs(cfg.dataDir)
	if err != nil {
		return nil, fmt.Errorf("cannot get absolute datadir path for %q: %v", cfg.dataDir, err)
	}

	p := &PostgresKeeper{
		cfg: cfg,

		bootUUID: common.UUID(),

		dataDir: dataDir,
		walDir:  cfg.walDir,

		pgListenAddress:       cfg.pgListenAddress,
		pgAdvertiseAddress:    cfg.pgAdvertiseAddress,
		pgPort:                cfg.pgPort,
		pgAdvertisePort:       cfg.pgAdvertisePort,
		pgBinPath:             cfg.pgBinPath,
		pgReplConnType:        cfg.pgReplConnType,
		pgReplAuthMethod:      cfg.pgReplAuthMethod,
		pgReplLocalAuthMethod: cfg.pgReplLocalAuthMethod,
		pgReplSslMode:         cfg.pgReplSslMode,
		pgReplUsername:        cfg.pgReplUsername,
		pgReplPassword:        cfg.pgReplPassword,
		pgSUConnType:          cfg.pgSUConnType,
		pgSUAuthMethod:        cfg.pgSUAuthMethod,
		pgSULocalAuthMethod:   cfg.pgSULocalAuthMethod,
		pgSUUsername:          cfg.pgSUUsername,
		pgSUPassword:          cfg.pgSUPassword,

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
		logger.Fatal().
			Str("saved uid", p.keeperLocalState.UID).
			Str("config uid", cfg.uid).
			Msg("saved uid differs from configuration uid")
	}
	if p.keeperLocalState.UID == "" {
		p.keeperLocalState.UID = cfg.uid
		if cfg.uid == "" {
			p.keeperLocalState.UID = common.UID()
			logger.Info().
				Str("uid", p.keeperLocalState.UID).
				Msg("uid generated")
		}
		if err = p.saveKeeperLocalState(); err != nil {
			logger.Info().
				AnErr("err", err).
				Msg("error while saving local state")
		}
	}

	logger.Info().Str("keeper uid", p.keeperLocalState.UID).Msg("")

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
	return p.pgSUUsername != "" && (p.pgSUPassword != "" || p.pgSUAuthMethod == authCert) && db.Spec.UsePgrewind
}

func (p *PostgresKeeper) updateKeeperInfo(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	p.localStateMutex.Lock()
	keeperUID := p.keeperLocalState.UID
	clusterUID := p.keeperLocalState.ClusterUID
	p.localStateMutex.Unlock()

	if clusterUID == "" {
		return nil
	}

	version, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just report maj and min as 0
		logger.Warn().AnErr("err", err).Msg("failed to get postgres binary version")
	}

	keeperInfo := &cluster.KeeperInfo{
		InfoUID:    common.UID(),
		UID:        keeperUID,
		ClusterUID: clusterUID,
		BootUUID:   p.bootUUID,
		PostgresBinaryVersion: cluster.PostgresBinaryVersion{
			Maj: int(version.Major()),
			Min: int(version.Minor()),
		},
		PostgresState: p.getLastPGState(ctx),

		CanBeMaster:             p.canBeMaster,
		CanBeSynchronousReplica: p.canBeSynchronousReplica,
	}

	// The time to live is just to automatically remove old entries, it's
	// not used to determine if the keeper info has been updated.
	return p.e.SetKeeperInfo(ctx, keeperUID, keeperInfo, p.sleepInterval)
}

func (p *PostgresKeeper) updatePGState(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	p.pgStateMutex.Lock()
	defer p.pgStateMutex.Unlock()
	pgState, err := p.GetPGState(ctx)
	if err != nil {
		logger.Error().AnErr("err", err).Msg("failed to get pg state")
	}
	p.lastPGState = pgState
}

// parseSynchronousStandbyNames extracts the standby names from the
// "synchronous_standby_names" postgres parameter.
//
// Since postgres 9.6 (https://www.postgresql.org/docs/9.6/static/runtime-config-replication.html)
// `synchronous_standby_names` can be in one of two formats:
//
//	num_sync ( standby_name [, ...] )
//	standby_name [, ...]
//
// two examples for this:
//
//	2 (node1,node2)
//	node1,node2
//
// TODO(sgotti) since postgres 10 (https://www.postgresql.org/docs/10/static/runtime-config-replication.html)
// `synchronous_standby_names` can be in one of three formats:
//
//	[FIRST] num_sync ( standby_name [, ...] )
//	ANY num_sync ( standby_name [, ...] )
//	standby_name [, ...]
//
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
				return nil, errors.New("synchronous standby string has number but lacks brackets")
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

// GetInSyncStandbys is a function that returns the InSyncStandbys
func (p *PostgresKeeper) GetInSyncStandbys(ctx context.Context) ([]string, error) {
	inSyncStandbysFullName, err := p.pgm.GetSyncStandbys(ctx)
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

// GetPGState is a function that returns the state of the postgresserver
func (p *PostgresKeeper) GetPGState(ctx context.Context) (*cluster.PostgresState, error) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
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
		pgParameters, err := p.pgm.GetConfigFilePGParameters(ctx)
		if err != nil {
			logger.Error().AnErr("err", err).Msg("cannot get configured pg parameters")
			return pgState, nil
		}
		logger.Debug().Any("pgParameters", pgParameters).Msg("got configured pg parameters")
		filteredPGParameters := common.Parameters{}
		for k, v := range pgParameters {
			if !util.StringInSlice(managedPGParameters, k) {
				filteredPGParameters[k] = v
			}
		}
		logger.Debug().
			Any("filteredPGParameters", filteredPGParameters).
			Msg("filtered out managed pg parameters")
		pgState.PGParameters = filteredPGParameters

		inSyncStandbys, err := p.GetInSyncStandbys(ctx)
		if err != nil {
			logger.Error().
				AnErr("err", err).
				Msg("failed to retrieve current in sync standbys from instance")
			return pgState, nil
		}

		pgState.SynchronousStandbys = inSyncStandbys

		sd, err := p.pgm.GetSystemData(ctx)
		if err != nil {
			logger.Error().AnErr("err", err).Msg("error getting pg state")
			return pgState, nil
		}
		pgState.SystemID = sd.SystemID
		pgState.TimelineID = sd.TimelineID
		pgState.XLogPos = sd.XLogPos

		ctlsh, err := getTimeLinesHistory(ctx, pgState, p.pgm, maxPostgresTimelinesHistory)
		if err != nil {
			logger.Error().AnErr("err", err).Msg("error getting timeline history")
			return pgState, nil
		}
		pgState.TimelinesHistory = ctlsh

		ow, err := p.pgm.OlderWalFile()
		if err != nil {
			logger.Warn().AnErr("err", err).Msg("error getting older wal file")
		} else {
			logger.Warn().Str("filename", ow).Msg("older wal file")
			pgState.OlderWalFile = ow
		}
		pgState.Healthy = true
	}

	return pgState, nil
}

func getTimeLinesHistory(
	ctx context.Context,
	pgState *cluster.PostgresState,
	pgm pg.PGManager,
	maxPostgresTimelinesHistory int) (
	cluster.PostgresTimelinesHistory,
	error) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	ctlsh := cluster.PostgresTimelinesHistory{}
	// if timeline <= 1 then no timeline history file exists.
	if pgState.TimelineID > 1 {
		var tlsh []*pg.TimelineHistory
		tlsh, err := pgm.GetTimelinesHistory(ctx, pgState.TimelineID)
		if err != nil {
			logger.Error().AnErr("err", err).Msg("error getting timeline history")
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

func (p *PostgresKeeper) getLastPGState(ctx context.Context) *cluster.PostgresState {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	p.pgStateMutex.Lock()
	pgState := p.lastPGState.DeepCopy()
	p.pgStateMutex.Unlock()
	logger.Debug().Str("pgstate dump", spew.Sdump(pgState)).Msg("")
	return pgState
}

// Start is function that starts PostgresKeeper
func (p *PostgresKeeper) Start(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	endSMCh := make(chan struct{})
	endPgStatecheckerCh := make(chan struct{})
	endUpdateKeeperInfo := make(chan struct{})

	var err error
	var cd *cluster.Data
	cd, _, err = p.e.GetClusterData(ctx)
	if err != nil {
		logger.Error().AnErr("err", err).Msg("error retrieving cluster data")
	} else if cd != nil {
		if cd.FormatVersion != cluster.CurrentCDFormatVersion {
			logger.Error().Uint64("version", cd.FormatVersion).Msg("unsupported clusterdata format version")
		} else if cd.Cluster != nil {
			p.sleepInterval = cd.Cluster.DefSpec().SleepInterval.Duration
			p.requestTimeout = cd.Cluster.DefSpec().RequestTimeout.Duration
		}
	}

	logger.Debug().Any("cd dump", cd).Msg("")

	// TODO(sgotti) reconfigure the various configurations options
	// (RequestTimeout) after a changed cluster config
	pgm := pg.NewManager(
		p.pgBinPath,
		p.dataDir,
		p.walDir,
		p.getLocalConnParams(),
		p.getLocalReplConnParams(),
		p.pgSUAuthMethod,
		p.pgSUUsername,
		p.pgSUPassword,
		p.pgReplAuthMethod,
		p.pgReplUsername,
		p.pgReplPassword,
		p.requestTimeout)

	p.pgm = pgm

	_ = p.pgm.StopIfStarted(ctx, true)

	smTimerCh := time.NewTimer(0).C
	updatePGStateTimerCh := time.NewTimer(0).C
	updateKeeperInfoTimerCh := time.NewTimer(0).C
	for {
		// The sleepInterval can be updated during normal execution. Ensure we regularly
		// refresh the metric to account for those changes.
		sleepInterval.Set(float64(p.sleepInterval / time.Second))

		select {
		case <-ctx.Done():
			logger.Debug().Msg("stopping stolon keeper")
			if err = p.pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgPgInst)
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
				if err := p.updateKeeperInfo(ctx); err != nil {
					logger.Error().AnErr("err", err).Msg("failed to update keeper info")
				}
				endUpdateKeeperInfo <- struct{}{}
			}()

		case <-endUpdateKeeperInfo:
			updateKeeperInfoTimerCh = time.NewTimer(p.sleepInterval).C
		}
	}
}

func (p *PostgresKeeper) resync(ctx context.Context, db, masterDB, followedDB *cluster.DB, tryPgrewind bool) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	pgm := p.pgm
	replConnParams := p.getReplConnParams(db, followedDB)
	standbySettings := &cluster.StandbySettings{
		PrimaryConninfo: replConnParams.ConnString(),
		PrimarySlotName: common.StolonName(db.UID)}

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
		err := pgm.SyncFromFollowedPGRewind(ctx, connParams, p.pgSUPassword)
		logger.Info().
			Str("masterDB", masterDB.UID).
			Str("keeper", followedDB.Spec.KeeperUID).
			Msg("syncing using pg_rewind")
		if err == nil {
			pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))
			return nil
		}
		// log pg_rewind error and fallback to pg_basebackup
		logger.Error().AnErr("err", err).Msg("error syncing with pg_rewind")
	}

	version, err := p.pgm.BinaryVersion()
	if err != nil {
		// in case we fail to parse the binary version then log it and just don't use replSlot
		logger.Warn().AnErr("err", err).Msg("failed to get postgres binary version")
	}
	replSlot := ""
	if version.GreaterThanEqual(pg.V96) {
		replSlot = common.StolonName(db.UID)
	}

	if err := pgm.RemoveAllIfInitialized(ctx); err != nil {
		return fmt.Errorf("failed to remove the postgres data dir: %v", err)
	}
	if logger.GetLevel() == zerolog.DebugLevel {
		logger.Debug().
			Str(followedStr, followedDB.UID).
			Str("keeper", followedDB.Spec.KeeperUID).
			Any("replConnParams", replConnParams).
			Msg("syncing from followed db")
	} else {
		logger.Info().
			Str(followedStr, followedDB.UID).
			Str("keeper", followedDB.Spec.KeeperUID).
			Msg("syncing from followed db")
	}

	if err := pgm.SyncFromFollowed(ctx, replConnParams, replSlot); err != nil {
		return fmt.Errorf("sync error: %v", err)
	}
	logger.Info().Msg("sync succeeded")

	pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))

	return nil
}

// TODO(sgotti) unify this with the sentinel one. They have the same logic but one uses *cluster.PostgresState
// while the other *cluster.DB
func (p *PostgresKeeper) isDifferentTimelineBranch(
	ctx context.Context, followedDB *cluster.DB, pgState *cluster.PostgresState) bool {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	if followedDB.Status.TimelineID < pgState.TimelineID {
		logger.Info().
			Uint64("followedTimeline", followedDB.Status.TimelineID).
			Uint64("timeline", pgState.TimelineID).
			Msg("followed instance timeline < than our timeline")
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
		logger.Info().
			Uint64("followedTimeline", followedDB.Status.TimelineID).
			Uint64("followedXlogpos", ftlh.SwitchPoint).
			Uint64("timeline", pgState.TimelineID).
			Uint64("xlogpos", tlh.SwitchPoint).
			Msg("followed instance timeline forked at a different xlog pos than our timeline")
		return true
	}

	// followedDB.Status.TimelineID > pgState.TimelineID
	ftlh := followedDB.Status.TimelinesHistory.GetTimelineHistory(pgState.TimelineID)
	if ftlh != nil {
		if ftlh.SwitchPoint < pgState.XLogPos {
			logger.Info().
				Uint64("followedTimeline", followedDB.Status.TimelineID).
				Uint64("followedXlogpos", ftlh.SwitchPoint).
				Uint64("timeline", pgState.TimelineID).
				Uint64("xlogpos", pgState.XLogPos).
				Msg("followed instance timeline forked before our current state")
			return true
		}
	}
	return false
}

func (p *PostgresKeeper) updateReplSlots(
	ctx context.Context,
	curReplSlots []string,
	uid string,
	followersUIDs,
	additionalReplSlots []string) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
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
			logger.Info().Str("slot", slot).Msg("dropping replication slot")
			if err := p.pgm.DropReplicationSlot(ctx, slot); err != nil {
				logger.Error().AnErr("err", err).Str("slot", slot).Msg("failed to drop replication slot")
				// don't return the error but continue also if drop failed (standby still connected)
			}
		}
	}

	// Create internal replication slots
	for slot := range internalReplSlots {
		if !util.StringInSlice(curReplSlots, slot) {
			logger.Info().Str("slot", slot).Msg("creating replication slot")
			if err := p.pgm.CreateReplicationSlot(ctx, slot); err != nil {
				logger.Error().AnErr("err", err).Str("slot", slot).Msg("failed to create replication slot")
				return err
			}
		}
	}
	return nil
}

func (p *PostgresKeeper) refreshReplicationSlots(ctx context.Context, _ *cluster.Data, db *cluster.DB) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	var currentReplicationSlots []string
	currentReplicationSlots, err := p.pgm.GetReplicationSlots(ctx)
	if err != nil {
		logger.Error().AnErr("err", err).Msg("failed to get replication slots")
		return err
	}

	followersUIDs := db.Spec.Followers

	if err = p.updateReplSlots(
		ctx,
		currentReplicationSlots,
		db.UID,
		followersUIDs,
		db.Spec.AdditionalReplicationSlots); err != nil {
		logger.Error().AnErr("err", err).Msg("error updating replication slots")
		return err
	}

	return nil
}

func (p *PostgresKeeper) postgresKeeperSM(ctx context.Context) {
	ctx, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	e := p.e
	pgm := p.pgm

	cd, _, err := e.GetClusterData(ctx)
	if err != nil {
		logger.Error().AnErr("err", err).Msg("error retrieving cluster data")
		return
	}
	logger.Debug().Any("clusterdata", cd).Msg("cd dump: %s")

	if cd == nil {
		logger.Info().Msg("no cluster data available, waiting for it to appear")
		return
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		logger.Error().Uint64("version", cd.FormatVersion).Msg("unsupported clusterdata format version")
		return
	}
	if err = cd.Cluster.Spec.Validate(); err != nil {
		logger.Error().AnErr("err", err).Msg("clusterdata validation failed")
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
				logger.Error().AnErr("err", err).Msg("failed to save keeper local state")
				return
			}
		}
	}

	k, ok := cd.Keepers[p.keeperLocalState.UID]
	if !ok {
		logger.Info().Msg("our keeper data is not available, waiting for it to appear")
		return
	}

	db := cd.FindDB(k)
	if db == nil {
		logger.Info().Msg("no db assigned")
		if err = pgm.StopIfStarted(ctx, true); err != nil {
			logger.Error().AnErr("err", err).Msg(errorMsgPgInst)
		}
		return
	}

	if p.bootUUID != k.Status.BootUUID {
		logger.Info().
			Str("bootUUID", p.bootUUID).
			Str("clusterBootUUID", k.Status.BootUUID).
			Msg("our db boot UID is different than the cluster data one, waiting for it to be updated")
		if err = pgm.StopIfStarted(ctx, true); err != nil {
			logger.Error().AnErr("err", err).Msg(errorMsgPgInst)
		}
		return
	}

	// Generate hba auth from clusterData
	pgm.SetHba(p.generateHBA(ctx, cd, db, p.waitSyncStandbysSynced))

	var pgParameters common.Parameters

	dbls := p.dbLocalStateCopy()
	if dbls.Initializing {
		// If we are here this means that the db initialization or
		// resync has failed so we have to clean up stale data
		logger.Error().Msg("db failed to initialize or resync")

		if err = pgm.StopIfStarted(ctx, true); err != nil {
			logger.Error().AnErr("err", err).Msg(errorMsgPgInst)
			return
		}

		// Clean up cluster db datadir
		if err = pgm.RemoveAllIfInitialized(ctx); err != nil {
			logger.Error().AnErr("err", err).Msg("failed to remove the postgres data dir")
			return
		}
		// Reset current db local state since it's not valid anymore
		ndbls := &DBLocalState{
			UID:          "",
			Generation:   cluster.NoGeneration,
			Initializing: false,
		}
		if err = p.saveDBLocalState(ndbls); err != nil {
			logger.Error().AnErr("err", err).Msg(errorMsgDbState)
			return
		}
	}

	if p.dbLocalState.UID != db.UID {
		var initialized bool
		initialized, err = pgm.IsInitialized()
		if err != nil {
			logger.Error().AnErr("err", err).Msg("failed to detect if instance is initialized")
			return
		}
		logger.Info().
			Str("db", p.dbLocalState.UID).
			Str("cdDB", db.UID).
			Msg("current db UID different than cluster data db UID")

		pgm.SetRecoveryOptions(nil)
		p.waitSyncStandbysSynced = false

		switch db.Spec.InitMode {
		case cluster.NewDB:
			logger.Info().Msg("initializing the database cluster")
			ndbls := &DBLocalState{
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgDbState)
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(ctx, db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			initConfig := &pg.InitConfig{}

			if db.Spec.NewConfig != nil {
				initConfig.Locale = db.Spec.NewConfig.Locale
				initConfig.Encoding = db.Spec.NewConfig.Encoding
				initConfig.DataChecksums = db.Spec.NewConfig.DataChecksums
			}

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgPgInst)
				return
			}
			if err = pgm.RemoveAllIfInitialized(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to remove the postgres data dir")
				return
			}
			if err = pgm.Init(ctx, initConfig); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to initialize postgres database cluster")
				return
			}

			if err = pgm.StartTmpMerged(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to start instance")
				return
			}
			if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				logger.Error().AnErr("err", err).Msg("timeout waiting for instance to be ready")
				return
			}
			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters(ctx)
				if err != nil {
					logger.Error().AnErr("err", err).Msg("failed to retrieve postgres parameters")
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					logger.Error().AnErr("err", err).Msg(errorMsgDbState)
					return
				}
			}

			logger.Info().Msg("setting roles")
			if err = pgm.SetupRoles(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to setup roles")
				return
			}

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgPgInst)
				return
			}
		case cluster.PITRDB:
			logger.Info().Msg("restoring the database cluster")
			ndbls := &DBLocalState{
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgDbState)
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(ctx, db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgPgInst)
				return
			}
			if err = pgm.RemoveAllIfInitialized(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to remove the postgres data dir")
				return
			}
			logger.Info().Msg("executing DataRestoreCommand")
			if err = pgm.Restore(ctx, db.Spec.PITRConfig.DataRestoreCommand); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to restore postgres database cluster")
				return
			}

			recoveryMode := pg.RecoveryModeRecovery
			var standbySettings *cluster.StandbySettings
			if db.Spec.FollowConfig != nil && db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
				recoveryMode = pg.RecoveryModeStandby
				standbySettings = db.Spec.FollowConfig.StandbySettings
			}

			pgm.SetRecoveryOptions(p.createRecoveryOptions(
				recoveryMode,
				standbySettings,
				db.Spec.PITRConfig.ArchiveRecoverySettings,
				db.Spec.PITRConfig.RecoveryTargetSettings))

			if err = pgm.StartTmpMerged(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to start instance")
				return
			}

			if recoveryMode == pg.RecoveryModeRecovery {
				// wait for the db having replayed all the wals
				logger.Info().Msg("waiting for recovery to be completed")
				if err = pgm.WaitRecoveryDone(cd.Cluster.DefSpec().SyncTimeout.Duration); err != nil {
					logger.Error().AnErr("err", err).Msg("recovery not finished")
					return
				}
				logger.Info().Msg("recovery completed")
			}
			if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().SyncTimeout.Duration); err != nil {
				logger.Error().AnErr("err", err).Msg("timeout waiting for instance to be ready")
				return
			}

			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters(ctx)
				if err != nil {
					logger.Error().AnErr("err", err).Msg("failed to retrieve postgres parameters")
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					logger.Error().AnErr("err", err).Msg(errorMsgDbState)
					return
				}
			}

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgDbState)
				return
			}

		case cluster.ResyncDB:
			logger.Info().Msg("resyncing the database cluster")
			ndbls := &DBLocalState{
				// replace our current db uid with the required one.
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: true,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgDbState)
				return
			}

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgDbState)
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(ctx, db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			var systemID string
			if !initialized {
				logger.Info().Msg("database cluster not initialized")
			} else {
				systemID, err = pgm.GetSystemID()
				if err != nil {
					logger.Error().AnErr("err", err).Msg("error retrieving systemd ID")
					return
				}
			}

			followedUID := db.Spec.FollowConfig.DBUID
			followedDB, ok := cd.DBs[followedUID]
			if !ok {
				logger.Error().Str(followedStr, followedUID).Msg("no db data available for followed db")
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
				logger.Warn().Msg("no current master, disabling pg_rewind for this resync")
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
			if err = p.resync(ctx, db, masterDB, followedDB, tryPgrewind); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to resync from followed instance")
				return
			}
			if err = pgm.Start(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to start instance")
				return
			}

			if tryPgrewind {
				fullResync := false
				// if not accepting connection assume that it's blocked waiting for missing wal
				// (see above TODO), so do a full resync using pg_basebackup.
				if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
					// revive:disable-next-line
					logger.Error().Msg("pg_rewinded standby is not accepting connection. it's probably waiting for unavailable wals. Forcing a full resync")
					fullResync = true
				} else {
					// Check again if it was really synced
					var pgState *cluster.PostgresState
					pgState, err = p.GetPGState(ctx)
					if err != nil {
						logger.Error().AnErr("err", err).Msg("cannot get current pgstate")
						return
					}
					if p.isDifferentTimelineBranch(ctx, followedDB, pgState) {
						fullResync = true
					}
				}

				if fullResync {
					if err = pgm.StopIfStarted(ctx, true); err != nil {
						logger.Error().AnErr("err", err).Msg(errorMsgDbState)
						return
					}
					if err = p.resync(ctx, db, masterDB, followedDB, false); err != nil {
						logger.Error().AnErr("err", err).Msg("failed to resync from followed instance")
						return
					}
				}
			}

		case cluster.ExistingDB:
			ndbls := &DBLocalState{
				// replace our current db uid with the required one.
				UID: db.UID,
				// Set a no generation since we aren't already converged.
				Generation:   cluster.NoGeneration,
				Initializing: false,
			}
			if err = p.saveDBLocalState(ndbls); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgDbState)
				return
			}

			// create postgres parameters with empty InitPGParameters
			pgParameters = p.createPGParameters(ctx, db)
			// update pgm postgres parameters
			pgm.SetParameters(pgParameters)

			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgDbState)
				return
			}
			if err = pgm.StartTmpMerged(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to start instance")
				return
			}
			if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				logger.Error().AnErr("err", err).Msg("timeout waiting for instance to be ready")
				return
			}
			if db.Spec.IncludeConfig {
				pgParameters, err = pgm.GetConfigFilePGParameters(ctx)
				if err != nil {
					logger.Error().AnErr("err", err).Msg("failed to retrieve postgres parameters")
					return
				}
				ndbls.InitPGParameters = pgParameters
				if err = p.saveDBLocalState(ndbls); err != nil {
					logger.Error().AnErr("err", err).Msg(errorMsgDbState)
					return
				}
			}
			if err = pgm.StopIfStarted(ctx, true); err != nil {
				logger.Error().AnErr("err", err).Msg(errorMsgDbState)
				return
			}
		case cluster.NoDB:
			// revive:disable-next-line
			logger.Error().Msg("different local dbUID but init mode is none, this shouldn't happen. Something bad happened to the keeper data. Check that keeper data is on a persistent volume and that the keeper state files weren't removed")
			return
		default:
			logger.Error().Str("initMode", string(db.Spec.InitMode)).Msg("unknown db init mode")
			return
		}
	}

	initialized, err := pgm.IsInitialized()
	if err != nil {
		logger.Error().AnErr("err", err).Msg("failed to detect if instance is initialized")
		return
	}

	if initialized {
		var started bool
		started, err = pgm.IsStarted()
		if err != nil {
			// log error getting instance state but go ahead.
			logger.Error().AnErr("err", err).Msg("failed to retrieve instance status")
		}
		logger.Debug().Bool("initialized", true).Bool("started", started).Msg("db status")
	} else {
		logger.Debug().Bool("initialized", false).Bool("started", false).Msg("db status")
	}

	// create postgres parameters
	pgParameters = p.createPGParameters(ctx, db)
	// update pgm postgres parameters
	pgm.SetParameters(pgParameters)

	var localRole common.Role
	if !initialized {
		logger.Info().Msg("database cluster not initialized")
		localRole = common.RoleUndefined
	} else {
		localRole, err = pgm.GetRole()
		if err != nil {
			logger.Error().AnErr("err", err).Msg("error retrieving current pg role")
			return
		}
	}

	targetRole := db.Spec.Role
	logger.Debug().Str("targetRole", string(targetRole)).Msg("target role")

	// Set metrics to power alerts about mismatched roles
	setRole(localRoleGauge, &localRole)
	setRole(targetRoleGauge, &targetRole)

	switch targetRole {
	case common.RolePrimary:
		// We are the elected master
		logger.Info().Msg("our db requested role is master")
		if localRole == common.RoleUndefined {
			logger.Error().Msg("database cluster not initialized but requested role is master. This shouldn't happen!")
			return
		}

		pgm.SetRecoveryOptions(nil)

		started, err := pgm.IsStarted()
		if err != nil {
			logger.Error().AnErr("err", err).Msg("failed to retrieve instance status")
			return
		}
		if !started {
			// if we have syncrepl enabled and the postgres instance is stopped, before opening connections to normal
			// users wait for having the defined synchronousStandbys in sync state.
			if db.Spec.SynchronousReplication {
				p.waitSyncStandbysSynced = true
				// revive:disable-next-line
				logger.Info().Msg("not allowing connection as normal users since synchronous replication is enabled and instance was down")
				pgm.SetHba(p.generateHBA(ctx, cd, db, true))
			}

			if err = pgm.Start(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to start postgres")
				return
			}
			if err = pgm.WaitReady(ctx, cd.Cluster.DefSpec().DBWaitReadyTimeout.Duration); err != nil {
				logger.Error().AnErr("err", err).Msg("timeout waiting for instance to be ready")
				return
			}
		}

		if localRole == common.RoleReplica {
			logger.Info().Msg("promoting to master")
			if err = pgm.Promote(ctx); err != nil {
				logger.Error().AnErr("err", err).Msg("failed to promote instance")
				return
			}
		} else {
			logger.Info().Msg("already master")
		}

		if err := p.refreshReplicationSlots(ctx, cd, db); err != nil {
			logger.Error().AnErr("err", err).Msg("error updating replication slots")
			return
		}

	case common.RoleReplica:
		// We are a standby
		var standbySettings *cluster.StandbySettings
		switch db.Spec.FollowConfig.Type {
		case cluster.FollowTypeInternal:
			followedUID := db.Spec.FollowConfig.DBUID
			logger.Info().Str(followedStr, followedUID).Msg("our db requested role is standby")
			followedDB, ok := cd.DBs[followedUID]
			if !ok {
				logger.Error().Str(followedStr, followedUID).Msg("no db data available for followed db")
				return
			}
			replConnParams := p.getReplConnParams(db, followedDB)
			standbySettings = &cluster.StandbySettings{
				PrimaryConninfo: replConnParams.ConnString(),
				PrimarySlotName: common.StolonName(db.UID)}
		case cluster.FollowTypeExternal:
			standbySettings = db.Spec.FollowConfig.StandbySettings
		default:
			logger.Error().Str("followType", string(db.Spec.FollowConfig.Type)).Msg("unknown follow type")
			return
		}
		switch localRole {
		case common.RolePrimary:
			logger.Error().Msg("cannot move from master role to standby role")
			return
		case common.RoleReplica:
			logger.Info().Msg("already standby")
			started, err := pgm.IsStarted()
			if err != nil {
				logger.Error().AnErr("err", err).Msg("failed to retrieve instance status")
				return
			}
			if !started {
				pgm.SetRecoveryOptions(p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil))
				if err = pgm.Start(ctx); err != nil {
					logger.Error().AnErr("err", err).Msg("failed to start postgres")
					return
				}
			}

			// Update our primary_conninfo if replConnString changed
			switch db.Spec.FollowConfig.Type {
			case cluster.FollowTypeInternal:
				followedUID := db.Spec.FollowConfig.DBUID
				followedDB, ok := cd.DBs[followedUID]
				if !ok {
					logger.Error().Str(followedStr, followedUID).Msg("no db data available for followed db")
					return
				}
				newReplConnParams := p.getReplConnParams(db, followedDB)
				logger.Debug().Any("newReplConnParams", newReplConnParams).Msg("newReplConnParams")

				standbySettings := &cluster.StandbySettings{
					PrimaryConninfo: newReplConnParams.ConnString(),
					PrimarySlotName: common.StolonName(db.UID)}

				curRecoveryOptions := pgm.CurRecoveryOptions()
				newRecoveryOptions := p.createRecoveryOptions(pg.RecoveryModeStandby, standbySettings, nil, nil)

				// Update recovery conf if parameters has changed
				if !curRecoveryOptions.RecoveryParameters.Equals(newRecoveryOptions.RecoveryParameters) {
					logger.Info().
						Any("curRecoveryParameters", curRecoveryOptions.RecoveryParameters).
						Any("newRecoveryParameters", newRecoveryOptions.RecoveryParameters).
						Msg("recovery parameters changed, restarting postgres instance")
					pgm.SetRecoveryOptions(newRecoveryOptions)

					if err = pgm.Restart(ctx, true); err != nil {
						logger.Error().AnErr("err", err).Msg("failed to restart postgres instance")
						return
					}
				}

				if err = p.refreshReplicationSlots(ctx, cd, db); err != nil {
					logger.Error().AnErr("err", err).Msg("error updating replication slots")
				}

			case cluster.FollowTypeExternal:
				curRecoveryOptions := pgm.CurRecoveryOptions()
				newRecoveryOptions := p.createRecoveryOptions(
					pg.RecoveryModeStandby,
					db.Spec.FollowConfig.StandbySettings,
					db.Spec.FollowConfig.ArchiveRecoverySettings,
					nil)

				// Update recovery conf if parameters has changed
				if !curRecoveryOptions.RecoveryParameters.Equals(newRecoveryOptions.RecoveryParameters) {
					logger.Info().
						Any("curRecoveryParameters", curRecoveryOptions.RecoveryParameters).
						Any("newRecoveryParameters", newRecoveryOptions.RecoveryParameters).
						Msg("recovery parameters changed, restarting postgres instance")
					pgm.SetRecoveryOptions(newRecoveryOptions)

					if err = pgm.Restart(ctx, true); err != nil {
						logger.Error().AnErr("err", err).Msg("failed to restart postgres instance")
						return
					}
				}

				if err = p.refreshReplicationSlots(ctx, cd, db); err != nil {
					logger.Error().AnErr("err", err).Msg("error updating replication slots")
				}
			}

		case common.RoleUndefined:
			logger.Info().Msg("our db role is none")
			return
		}
	case common.RoleUndefined:
		logger.Info().Msg("our db requested role is none")
		return
	}

	// update pg parameters
	pgParameters = p.createPGParameters(ctx, db)

	// Log synchronous replication changes
	prevSyncStandbyNames := pgm.CurParameters()["synchronous_standby_names"]
	syncStandbyNames := pgParameters["synchronous_standby_names"]
	if db.Spec.SynchronousReplication {
		if prevSyncStandbyNames != syncStandbyNames {
			logger.Info().
				Str("prevSyncStandbyNames", prevSyncStandbyNames).
				Str("syncStandbyNames", syncStandbyNames).
				Msg("needed synchronous_standby_names changed")
		}
	} else {
		if prevSyncStandbyNames != "" {
			logger.Info().
				Str("syncStandbyNames", prevSyncStandbyNames).
				Msg("sync replication disabled, removing current synchronous_standby_names")
		}
	}

	needsReload := false
	changedParams := pgParameters.Diff(pgm.CurParameters())

	if !pgParameters.Equals(pgm.CurParameters()) {
		logger.Info().Msg("postgres parameters changed, reloading postgres instance")
		pgm.SetParameters(pgParameters)
		needsReload = true
	} else {
		// for tests
		logger.Info().Msg("postgres parameters not changed")
	}

	// Generate hba auth from clusterData

	// if we have syncrepl enabled and the postgres instance is stopped, before opening connections to normal users
	// wait for having the defined synchronousStandbys in sync state.
	if db.Spec.SynchronousReplication && p.waitSyncStandbysSynced {
		inSyncStandbys, err := p.GetInSyncStandbys(ctx)
		if err != nil {
			logger.Error().AnErr("err", err).Msg("failed to retrieve current in sync standbys from instance")
			return
		}
		if !util.CompareStringSliceNoOrder(inSyncStandbys, db.Spec.SynchronousStandbys) {
			// revive:disable-next-line
			logger.Info().Msg("not allowing connection as normal users since synchronous replication is enabled, instance was down and not all sync standbys are synced")
		} else {
			p.waitSyncStandbysSynced = false
		}
	} else {
		p.waitSyncStandbysSynced = false
	}
	newHBA := p.generateHBA(ctx, cd, db, p.waitSyncStandbysSynced)
	if !reflect.DeepEqual(newHBA, pgm.CurHba()) {
		logger.Info().Msg("postgres hba entries changed, reloading postgres instance")
		pgm.SetHba(newHBA)
		needsReload = true
	} else {
		// for tests
		logger.Info().Msg("postgres hba entries not changed")
	}

	if needsReload {
		needsReloadGauge.Set(1) // mark as reload needed
		if err := pgm.Reload(ctx); err != nil {
			logger.Error().AnErr("err", err).Msg("failed to reload postgres instance")
		} else {
			needsReloadGauge.Set(0) // successful reload implies no longer required
		}
	}

	{
		clusterSpec := cd.Cluster.DefSpec()
		automaticPgRestartEnabled := *clusterSpec.AutomaticPgRestart

		needsRestart, err := pgm.IsRestartRequired(ctx, changedParams)
		if err != nil {
			logger.Error().AnErr("err", err).Msg("failed to check if restart is required")
		}

		if needsRestart {
			needsRestartGauge.Set(1) // mark as restart needed
			if automaticPgRestartEnabled {
				logger.Info().Msg("restarting postgres")
				if err := pgm.Restart(ctx, true); err != nil {
					logger.Error().AnErr("err", err).Msg("failed to restart postgres instance")
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
		logger.Error().AnErr("err", err).Msg(errorMsgDbState)
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
	sj, err := os.ReadFile(p.keeperLocalStateFilePath())
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
	return common.WriteFileAtomic(p.keeperLocalStateFilePath(), ownerRWPermisions, sj)
}

func (p *PostgresKeeper) dbLocalStateFilePath() string {
	return filepath.Join(p.cfg.dataDir, "dbstate")
}

func (p *PostgresKeeper) loadDBLocalState() error {
	sj, err := os.ReadFile(p.dbLocalStateFilePath())
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
	if err = common.WriteFileAtomic(p.dbLocalStateFilePath(), ownerRWPermisions, sj); err != nil {
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
	case common.RolePrimary:
		return true
	case common.RoleReplica:
		if db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
			return true
		}
		return false
	default:
		panic("invalid db role in db Spec")
	}
}

// localAuthMethod returns the authentication method that works for local connections
// cert does not work for local connections, in which case we should fall back to peer authentication
func localAuthMethod(ctx context.Context, authMethod string) string {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	if authMethod == authCert {
		logger.Info().Msg("using peer instead of cert for local connection authentication method")
		return authPeer
	}
	return authMethod
}

// generateHBA generates the instance hba entries depending on the value of
// DefaultSUReplAccessMode.
// When onlyInternal is true only rules needed for replication will be setup
// and the traffic should be permitted only for pgSUUsername standard
// connections and pgReplUsername replication connections.
func (p *PostgresKeeper) generateHBA(ctx context.Context, cd *cluster.Data, db *cluster.DB,
	onlyInternal bool) []string {
	// Minimal entries for local normal and replication connections needed by the stolon keeper
	// Matched local connections are for postgres database and suUsername user with authMd5 auth
	// Matched local replication connections are for replUsername user with authMd5 auth
	computedHBA := []string{
		fmt.Sprintf("local postgres %s %s", p.pgSUUsername, localAuthMethod(ctx, p.pgSULocalAuthMethod)),
		fmt.Sprintf("local replication %s %s", p.pgReplUsername, localAuthMethod(ctx, p.pgReplLocalAuthMethod)),
	}

	switch *cd.Cluster.DefSpec().DefaultSUReplAccessMode {
	case cluster.SUReplAccessAll:
		// all the keepers will accept connections from every host
		computedHBA = append(
			computedHBA,
			fmt.Sprintf("%s all %s %s %s", p.pgSUConnType, p.pgSUUsername, "0.0.0.0/0", p.pgSUAuthMethod),
			fmt.Sprintf("%s all %s %s %s", p.pgSUConnType, p.pgSUUsername, "::0/0", p.pgSUAuthMethod),
			fmt.Sprintf("%s replication %s %s %s", p.pgReplConnType, p.pgReplUsername, "0.0.0.0/0", p.pgReplAuthMethod),
			fmt.Sprintf("%s replication %s %s %s", p.pgReplConnType, p.pgReplUsername, "::0/0", p.pgReplAuthMethod),
		)
	case cluster.SUReplAccessStrict:
		// only the master keeper (primary instance or standby of a remote primary when in standby cluster mode) will
		// accept connections only from the other standby keepers IPs
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
					fmt.Sprintf(
						"%s all %s %s/32 %s",
						p.pgSUConnType,
						p.pgSUUsername,
						address,
						p.pgReplAuthMethod),
					fmt.Sprintf(
						"%s replication %s %s/32 %s",
						p.pgReplConnType,
						p.pgReplUsername,
						address,
						p.pgReplAuthMethod),
				)
			}
		}
	}

	if !onlyInternal {
		// By default, if no custom pg_hba entries are provided, accept
		// connections for all databases and users with authMd5 auth
		if db.Spec.PGHBA != nil {
			computedHBA = append(computedHBA, db.Spec.PGHBA...)
		} else {
			computedHBA = append(
				computedHBA,
				fmt.Sprintf("%s all all 0.0.0.0/0 %s", p.pgSUConnType, p.pgSUAuthMethod),
				fmt.Sprintf("%s all all ::0/0 %s", p.pgSUConnType, p.pgSUAuthMethod),
			)
		}
	}

	// return generated Hba merged with user Hba
	return computedHBA
}

func sigHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	s := <-sigs
	logger.Debug().Any("signal", s).Msg("got signal")
	shutdownSeconds.SetToCurrentTime()
	cancel()
}

// Execute is the main executor of keeper file
func Execute() {
	_, logger := logging.GetLogComponent(context.Background(), logging.KeeperComponent)
	if err := flagutil.SetFlagsFromEnv(CmdKeeper.PersistentFlags(), "STKEEPER"); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}

	if err := CmdKeeper.Execute(); err != nil {
		logger.Fatal().AnErr("err", err).Msg("")
	}
}

func keeper(c *cobra.Command, _ []string) {
	ctx, logger := logging.GetLogComponent(context.Background(), logging.KeeperComponent)
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
			logger.Fatal().AnErr("err", err).Msg("cannot get current user")
		}
		cfg.pgSUUsername = user
	}

	validAuthMethods := map[string]struct{}{}
	validAuthMethods[authTrust] = struct{}{}
	validAuthMethods[authMd5] = struct{}{}
	validAuthMethods[authCert] = struct{}{}
	validAuthMethods[authIdent] = struct{}{}
	validAuthMethods[authPeer] = struct{}{}
	validConnectionTypes := map[string]struct{}{}
	validConnectionTypes[connTypeHost] = struct{}{}
	validConnectionTypes[connTypeHostSsl] = struct{}{}
	validConnectionTypes[connTypeHostNoSsl] = struct{}{}
	validConnectionTypes[connTypeHostGssEnc] = struct{}{}
	validConnectionTypes[connTypeHostNoGssEnc] = struct{}{}
	logging.SetStaticLevel(cfg.LogLevel)
	if cfg.debug {
		logging.SetStaticLevel("debug")
	}
	if cmd.IsColorLoggerEnable(c, &cfg.CommonConfig) {
		logging.EnableColor()
	}

	if cfg.dataDir == "" {
		logger.Fatal().Msg("data dir required")
	}

	if err = cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		logger.Fatal().Msg(err.Error())
	}

	cmd.SetMetrics(&cfg.CommonConfig, "keeper")

	if err = os.MkdirAll(cfg.dataDir, ownerRWExecPermisions); err != nil {
		logger.Fatal().AnErr("err", err).Msg("cannot create data dir")
	}

	if cfg.pgListenAddress == "" {
		logger.Fatal().Msg("--pg-listen-address is required")
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
		logger.Warn().
			Str("flag", listenAddFlag).
			Str("value", cfg.pgAdvertiseAddress).
			Msg("provided flag is not an ip address but a hostname. " +
				"This will be advertized to the other components and may have undefined behaviors " +
				"if resolved differently by other hosts")
	}

	ipAddr, err := net.ResolveIPAddr("ip", cfg.pgAdvertiseAddress)
	if err != nil {
		logger.Warn().
			AnErr("err", err).
			Str("flag", listenAddFlag).
			Str("address", cfg.pgAdvertiseAddress).
			Msg("cannot resolve provided flag")
	} else {
		if ipAddr.IP.IsLoopback() {
			logger.Warn().
				Str("flag", listenAddFlag).
				Str("address", cfg.pgAdvertiseAddress).
				Msg("provided address for flag is a loopback ip. " +
					"This will be advertized to the other components " +
					"and communication will fail if they are on different hosts")
		}
	}

	if cfg.pgSULocalAuthMethod == "" {
		cfg.pgSULocalAuthMethod = cfg.pgSUAuthMethod
	}
	if cfg.pgReplLocalAuthMethod == "" {
		cfg.pgReplLocalAuthMethod = cfg.pgReplAuthMethod
	}
	if _, ok := validConnectionTypes[cfg.pgReplConnType]; !ok {
		logger.Fatal().
			Msg("--pg-repl-connection-type must be one of: host, hostssl, hostnossl, hostgssenc, hostnogssenc")
	}
	if _, ok := validConnectionTypes[cfg.pgSUConnType]; !ok {
		logger.Fatal().
			Msg("--pg-su-connection-type must be one of: host, hostssl, hostnossl, hostgssenc, hostnogssenc")
	}
	if _, ok := validAuthMethods[cfg.pgReplAuthMethod]; !ok {
		logger.Fatal().
			Msg("--pg-repl-auth-method must be one of: ident, authMd5, password, trust or cert")
	}
	if cfg.pgReplUsername == "" {
		logger.Fatal().
			Msg("--pg-repl-username is required")
	}
	if _, ok := validAuthMethods[cfg.pgReplLocalAuthMethod]; !ok {
		logger.Fatal().
			Msg("--pg-repl-local-auth-method must be one of: ident, authMd5, password, trust or cert")
	}
	if cfg.pgReplAuthMethod == authTrust {
		logger.Warn().
			Msg("not utilizing a password for replication between hosts is extremely dangerous")
		if cfg.pgReplPassword != "" || cfg.pgReplPasswordFile != "" {
			// revive:disable-next-line
			logger.Fatal().
				Msg("can not utilize --pg-repl-auth-method trust together with --pg-repl-password " +
					"or --pg-repl-passwordfile")
		}
	}
	if cfg.pgSUAuthMethod == authTrust || cfg.pgSULocalAuthMethod == authTrust {
		logger.Warn().Msg("not utilizing a password for superuser is extremely dangerous")
		if (cfg.pgSUAuthMethod == authTrust ||
			cfg.pgSULocalAuthMethod == authTrust) && (cfg.pgSUPassword != "" ||
			cfg.pgSUPasswordFile != "") {
			// revive:disable-next-line
			logger.Fatal().Msg("can not utilize --pg-su-auth-method trust and --pg-su-auth-method trust together with --pg-su-password or --pg-su-passwordfile")
		}
	}
	if cfg.pgReplAuthMethod == authMd5 && cfg.pgReplPassword == "" && cfg.pgReplPasswordFile == "" {
		logger.Fatal().Msg("one of --pg-repl-password or --pg-repl-passwordfile is required")
	}
	if cfg.pgReplAuthMethod == authMd5 && cfg.pgReplPassword != "" && cfg.pgReplPasswordFile != "" {
		logger.Fatal().Msg("only one of --pg-repl-password or --pg-repl-passwordfile must be provided")
	}
	if _, ok := validAuthMethods[cfg.pgSUAuthMethod]; !ok {
		logger.Fatal().Msg("--pg-su-auth-method must be one of: ident, authMd5, password, trust or cert")
	}
	if cfg.pgSUAuthMethod == authMd5 && cfg.pgSUPassword == "" && cfg.pgSUPasswordFile == "" {
		logger.Fatal().Msg("one of --pg-su-password or --pg-su-passwordfile is required")
	}
	if cfg.pgSUAuthMethod == authMd5 && cfg.pgSUPassword != "" && cfg.pgSUPasswordFile != "" {
		logger.Fatal().Msg("only one of --pg-su-password or --pg-su-passwordfile must be provided")
	}
	if _, ok := validAuthMethods[cfg.pgSULocalAuthMethod]; !ok {
		logger.Fatal().
			Msg("--pg-su-local-auth-method must be one of: ident, authMd5, password, trust or cert")
	}

	if cfg.pgReplPasswordFile != "" {
		cfg.pgReplPassword, err = readPasswordFromFile(ctx, cfg.pgReplPasswordFile)
		if err != nil {
			logger.Fatal().
				AnErr("err", err).
				Msg("cannot read pg replication user password")
		}
	}
	if cfg.pgSUPasswordFile != "" {
		cfg.pgSUPassword, err = readPasswordFromFile(ctx, cfg.pgSUPasswordFile)
		if err != nil {
			logger.Fatal().AnErr("err", err).Msg("cannot read pg superuser password")
		}
	}

	// Trim trailing new lines from passwords
	tp := strings.TrimRight(cfg.pgSUPassword, "\r\n")
	if cfg.pgSUPassword != tp {
		logger.Warn().Msg("superuser password contain trailing new line, removing")
		if tp == "" {
			logger.Fatal().Msg("superuser password is empty after removing trailing new line")
		}
		cfg.pgSUPassword = tp
	}

	tp = strings.TrimRight(cfg.pgReplPassword, "\r\n")
	if cfg.pgReplPassword != tp {
		logger.Warn().Msg("replication user password contain trailing new line, removing")
		if tp == "" {
			logger.Fatal().Msg("replication user password is empty after removing trailing new line")
		}
		cfg.pgReplPassword = tp
	}

	if cfg.pgSUUsername == cfg.pgReplUsername {
		logger.Warn().
			Msg("superuser name and replication user name are the same. Different users are suggested.")
		if cfg.pgReplAuthMethod != cfg.pgSUAuthMethod {
			logger.Fatal().Msg("do not support different auth methods when utilizing superuser for replication.")
		}
		if cfg.pgSUPassword != cfg.pgReplPassword && cfg.pgSUAuthMethod == authMd5 && cfg.pgReplAuthMethod == authMd5 {
			// revive:disable-next-line
			logger.Fatal().Msg("provided superuser name and replication user name are the same but provided passwords are different")
		}
	}

	// Open (and create if needed) the lock file.
	// There is no need to clean up this file since we don't use the file as an actual lock. We get a lock
	// on the file. So the lock get released when our process stops (or logger.Fatal).
	var lockFile *os.File
	if !cfg.disableDataDirLocking {
		lockFileName := filepath.Join(cfg.dataDir, "lock")
		lockFile, err = os.OpenFile(lockFileName, os.O_RDWR|os.O_CREATE, ownerRWOtherRPermisions)
		if err != nil {
			logger.Fatal().
				Str("logfile", lockFileName).
				AnErr("err", err).
				Msg("cannot take exclusive lock on data dir")
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
			logger.Fatal().
				Str("logfile", lockFileName).
				AnErr("err", err).
				Msg("cannot take exclusive lock on data dir")
		}

		logger.Info().Msg("exclusive lock on data dir taken")
	}

	if cfg.uid != "" {
		if !pg.IsValidReplSlotName(cfg.uid) {
			// revive:disable-next-line
			logger.Fatal().
				Str("uid", cfg.uid).
				Msg("keeper uid not valid. " +
					"It can contain only lower-case letters, numbers and the underscore character")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	end := make(chan error)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go sigHandler(ctx, sigs, cancel)

	if cfg.MetricsListenAddress != "" {
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			err = http.ListenAndServe(cfg.MetricsListenAddress, nil)
			if err != nil {
				logger.Error().AnErr("err", err).Msg("metrics http server error")
				cancel()
			}
		}()
	}

	p, err := NewPostgresKeeper(ctx, &cfg, end)
	if err != nil {
		logger.Fatal().AnErr("err", err).Msg("cannot create keeper")
	}
	go p.Start(ctx)

	<-end

	if !cfg.disableDataDirLocking {
		if err := lockFile.Close(); err != nil {
			logger.Fatal().AnErr("err", err).Msg("closing lock file failed")
		}
	}
}
