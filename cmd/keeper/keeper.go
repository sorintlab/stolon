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
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/flagutil"
	"github.com/sorintlab/stolon/pkg/kubernetes"
	"github.com/sorintlab/stolon/pkg/postgresql"
	pg "github.com/sorintlab/stolon/pkg/postgresql"
	"github.com/sorintlab/stolon/pkg/store"
	"github.com/sorintlab/stolon/pkg/util"

	"github.com/coreos/pkg/capnslog"
	"github.com/coreos/rkt/pkg/lock"
	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

var log = capnslog.NewPackageLogger("github.com/sorintlab/stolon/cmd", "keeper")

func init() {
	capnslog.SetFormatter(capnslog.NewPrettyFormatter(os.Stderr, true))
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
}

var cmdKeeper = &cobra.Command{
	Use: "stolon-keeper",
	Run: keeper,
}

type KeeperLocalState struct {
	UID string
}

type DBLocalState struct {
	UID        string
	Generation int64
	// Initializing registers when the db is initializing. Needed to detect
	// when the initialization has failed.
	Initializing bool
}

type config struct {
	id                      string
	storeBackend            string
	storeEndpoints          string
	dataDir                 string
	clusterName             string
	listenAddress           string
	port                    string
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
		log.Fatalf("%v", err)
	}

	cmdKeeper.PersistentFlags().StringVar(&cfg.id, "id", "", "keeper id (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random id will be generated.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.storeBackend, "store-backend", "", "store backend type (etcd or consul)")
	cmdKeeper.PersistentFlags().StringVar(&cfg.storeEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (defaults: 127.0.0.1:2379 for etcd, 127.0.0.1:8500 for consul)")
	cmdKeeper.PersistentFlags().StringVar(&cfg.dataDir, "data-dir", "", "data directory")
	cmdKeeper.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdKeeper.PersistentFlags().StringVar(&cfg.listenAddress, "listen-address", "localhost", "keeper listening address")
	cmdKeeper.PersistentFlags().StringVar(&cfg.port, "port", "5431", "keeper listening port")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgListenAddress, "pg-listen-address", "localhost", "postgresql instance listening address")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgPort, "pg-port", "5432", "postgresql instance listening port")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgBinPath, "pg-bin-path", "", "absolute path to postgresql binaries. If empty they will be searched in the current PATH")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgReplUsername, "pg-repl-username", "", "postgres replication user name. Required. It'll be created on db initialization. Requires --pg-repl-password or --pg-repl-passwordfile. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgReplPassword, "pg-repl-password", "", "postgres replication user password. Required. Only one of --pg-repl-password or --pg-repl-passwordfile is required. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgReplPasswordFile, "pg-repl-passwordfile", "", "postgres replication user password file. Only one of --pg-repl-password or --pg-repl-passwordfile is required. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgSUUsername, "pg-su-username", user, "postgres superuser user name. Used for keeper managed instance access and pg_rewind based synchronization. It'll be created on db initialization. Defaults to the name of the effective user running stolon-keeper. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgSUPassword, "pg-su-password", "", "postgres superuser password. Needed for pg_rewind based synchronization. If provided it'll be configured on db initialization. Only one of --pg-su-password or --pg-su-passwordfile is required. Must be the same for all keepers.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgSUPasswordFile, "pg-su-passwordfile", "", "postgres superuser password file. Requires --pg-su-username. Must be the same for all keepers)")
	cmdKeeper.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
}

var mandatoryPGParameters = common.Parameters{
	"unix_socket_directories": "/tmp",
	"wal_level":               "hot_standby",
	"wal_keep_segments":       "8",
	"hot_standby":             "on",
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

func readPasswordFromFile(filepath string) (string, error) {
	fi, err := os.Lstat(filepath)
	if err != nil {
		return "", fmt.Errorf("unable to read password from file %s: %v", filepath, err)
	}

	if fi.Mode() > 0600 {
		//TODO: enforce this by exiting with an error. Kubernetes makes this file too open today.
		log.Warningf("password file %s permissions %#o are too open. This file should only be readable to the user executing stolon! Continuing...", filepath, fi.Mode())
	}

	pwBytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return "", fmt.Errorf("unable to read password from file %s: %v", filepath, err)
	}
	return strings.TrimSpace(string(pwBytes)), nil
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
		"host":     "localhost",
		"port":     p.pgPort,
		"dbname":   "postgres",
		"sslmode":  "disable",
	}
}

func (p *PostgresKeeper) getOurReplConnParams() pg.ConnParams {
	return pg.ConnParams{
		"user":     p.pgReplUsername,
		"password": p.pgReplPassword,
		"host":     p.pgListenAddress,
		"port":     p.pgPort,
		"sslmode":  "disable",
	}
}

func (p *PostgresKeeper) createPGParameters(db *cluster.DB) common.Parameters {
	parameters := common.Parameters{}
	// Copy user defined pg parameters
	for k, v := range db.Spec.PGParameters {
		parameters[k] = v
	}

	// Add/Replace mandatory PGParameters
	for k, v := range mandatoryPGParameters {
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
	parameters["max_wal_senders"] = strconv.FormatUint(uint64(db.Spec.MaxStandbys+2), 10)

	// required by pg_rewind (if data checksum is enabled it's ignored)
	if db.Spec.UsePgrewind {
		parameters["wal_log_hints"] = "on"
	}

	// Setup synchronous replication
	if db.Spec.SynchronousReplication {
		standbyNames := []string{}
		for _, follower := range db.Spec.Followers {
			standbyNames = append(standbyNames, common.StolonName(follower))
		}
		parameters["synchronous_standby_names"] = strings.Join(standbyNames, ",")
	} else {
		parameters["synchronous_standby_names"] = ""
	}

	return parameters
}

type PostgresKeeper struct {
	cfg *config

	dataDir             string
	storeBackend        string
	storeEndpoints      string
	listenAddress       string
	port                string
	debug               bool
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

	kvstore, err := store.NewStore(store.Backend(cfg.storeBackend), cfg.storeEndpoints)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}
	e := store.NewStoreManager(kvstore, storePath)

	p := &PostgresKeeper{
		cfg: cfg,

		dataDir:        cfg.dataDir,
		storeBackend:   cfg.storeBackend,
		storeEndpoints: cfg.storeEndpoints,

		listenAddress:       cfg.listenAddress,
		port:                cfg.port,
		debug:               cfg.debug,
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
	if p.keeperLocalState.UID != "" && p.cfg.id != "" && p.keeperLocalState.UID != p.cfg.id {
		log.Fatalf("saved id %q differs from configuration id: %q", p.keeperLocalState.UID, cfg.id)
	}
	if p.keeperLocalState.UID == "" {
		p.keeperLocalState.UID = cfg.id
		if cfg.id == "" {
			p.keeperLocalState.UID = common.UID()
			log.Infof("generated id: %s", p.keeperLocalState.UID)
		}
		if err = p.saveKeeperLocalState(); err != nil {
			log.Fatalf("error: %v", err)
		}
	}

	log.Infof("id: %s", p.keeperLocalState.UID)

	err = p.loadDBLocalState()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load db local state file: %v", err)
	}
	return p, nil
}

func (p *PostgresKeeper) usePgrewind(db *cluster.DB) bool {
	return p.pgSUUsername != "" && p.pgSUPassword != "" && db.Spec.UsePgrewind
}

func (p *PostgresKeeper) publish() error {
	discoveryInfo := &cluster.KeeperDiscoveryInfo{
		ListenAddress: p.listenAddress,
		Port:          p.port,
	}
	log.Debugf(spew.Sprintf("discoveryInfo: %#v", discoveryInfo))

	p.localStateMutex.Lock()
	keeperUID := p.keeperLocalState.UID
	p.localStateMutex.Unlock()
	if err := p.e.SetKeeperDiscoveryInfo(keeperUID, discoveryInfo, p.sleepInterval*2); err != nil {
		return err
	}
	return nil
}

func (p *PostgresKeeper) infoHandler(w http.ResponseWriter, req *http.Request) {
	p.localStateMutex.Lock()
	keeperUID := p.keeperLocalState.UID
	p.localStateMutex.Unlock()

	keeperInfo := cluster.KeeperInfo{
		UID:           keeperUID,
		ListenAddress: p.listenAddress,
		Port:          p.port,
	}

	if err := json.NewEncoder(w).Encode(&keeperInfo); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (p *PostgresKeeper) pgStateHandler(w http.ResponseWriter, req *http.Request) {
	pgState := p.getLastPGState()

	if pgState == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(pgState); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (p *PostgresKeeper) updatePGState(pctx context.Context) {
	p.pgStateMutex.Lock()
	defer p.pgStateMutex.Unlock()
	pgState, err := p.GetPGState(pctx)
	if err != nil {
		log.Errorf("failed to get pg state: %v", err)
		return
	}
	p.lastPGState = pgState
}

func (p *PostgresKeeper) GetPGState(pctx context.Context) (*cluster.PostgresState, error) {
	p.getPGStateMutex.Lock()
	defer p.getPGStateMutex.Unlock()
	// Just get one pgstate at a time to avoid exausting available connections
	pgState := &cluster.PostgresState{}
	pgState.Healthy = true

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
			log.Errorf("error: %v", err)
			return pgState, nil
		}
		log.Debugf("pgParameters: %v", pgParameters)
		filteredPGParameters := common.Parameters{}
		for k, v := range pgParameters {
			if !util.StringInSlice(managedPGParameters, k) {
				filteredPGParameters[k] = v
			}
		}
		log.Debugf("filteredPGParameters: %v", filteredPGParameters)
		pgState.PGParameters = filteredPGParameters

		sd, err := p.pgm.GetSystemData()
		if err != nil {
			pgState.Healthy = false
			log.Errorf("error getting pg state: %v", err)
			return pgState, nil
		}
		pgState.SystemID = sd.SystemID
		pgState.TimelineID = sd.TimelineID
		pgState.XLogPos = sd.XLogPos

		// if timeline <= 1 then no timeline history file exists.
		pgState.TimelinesHistory = cluster.PostgresTimelinesHistory{}
		if pgState.TimelineID > 1 {
			tlsh, err := p.pgm.GetTimelinesHistory(pgState.TimelineID)
			if err != nil {
				pgState.Healthy = false
				log.Errorf("error getting timeline history: %v", err)
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
	}

	return pgState, nil
}

func (p *PostgresKeeper) getLastPGState() *cluster.PostgresState {
	p.pgStateMutex.Lock()
	pgState := p.lastPGState.Copy()
	p.pgStateMutex.Unlock()
	log.Debugf("pgstate: %s", spew.Sdump(pgState))
	return pgState
}

func (p *PostgresKeeper) Start() {
	endSMCh := make(chan struct{})
	endPgStatecheckerCh := make(chan struct{})
	endPublish := make(chan struct{})
	endApiCh := make(chan error)

	var err error
	var cd *cluster.ClusterData
	cd, _, err = p.e.GetClusterData()
	if err != nil {
		log.Errorf("error retrieving cluster data: %v", err)
	} else if cd != nil {
		if cd.FormatVersion != cluster.CurrentCDFormatVersion {
			log.Errorf("unsupported clusterdata format version %d", cd.FormatVersion)
		} else if cd.Cluster != nil {
			p.sleepInterval = cd.Cluster.Spec.SleepInterval.Duration
			p.requestTimeout = cd.Cluster.Spec.RequestTimeout.Duration
		}
	}

	log.Debugf("cluster data: %s", spew.Sdump(cd))

	// TODO(sgotti) reconfigure the various configurations options
	// (RequestTimeout) after a changed cluster config
	pgParameters := make(common.Parameters)
	pgm := postgresql.NewManager(p.pgBinPath, p.dataDir, pgParameters, p.getLocalConnParams(), p.getOurReplConnParams(), p.pgSUUsername, p.pgSUPassword, p.pgReplUsername, p.pgReplPassword, p.requestTimeout)
	p.pgm = pgm

	p.pgm.Stop(true)

	http.HandleFunc("/info", p.infoHandler)
	http.HandleFunc("/pgstate", p.pgStateHandler)
	go func() {
		endApiCh <- http.ListenAndServe(fmt.Sprintf("%s:%s", p.listenAddress, p.port), nil)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	smTimerCh := time.NewTimer(0).C
	updatePGStateTimerCh := time.NewTimer(0).C
	publishCh := time.NewTimer(0).C
	for true {
		select {
		case <-p.stop:
			log.Debugf("stopping stolon keeper")
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
			go func() {
				p.updatePGState(ctx)
				endPgStatecheckerCh <- struct{}{}
			}()

		case <-endPgStatecheckerCh:
			updatePGStateTimerCh = time.NewTimer(p.sleepInterval).C

		case <-publishCh:
			go func() {
				p.publish()
				endPublish <- struct{}{}
			}()

		case <-endPublish:
			publishCh = time.NewTimer(p.sleepInterval).C

		case err := <-endApiCh:
			if err != nil {
				log.Fatal("ListenAndServe: ", err)
			}
			close(p.stop)
		}
	}
}

func (p *PostgresKeeper) resync(db, followedDB *cluster.DB, tryPgrewind, started bool) error {
	pgm := p.pgm
	if started {
		if err := pgm.Stop(true); err != nil {
			return fmt.Errorf("failed to stop pg instance: %v", err)
		}
	}
	replConnParams := p.getReplConnParams(db, followedDB)

	// TODO(sgotti) Actually we don't check if pg_rewind is installed or if
	// postgresql version is > 9.5 since someone can also use an externally
	// installed pg_rewind for postgres 9.4. If a pg_rewind executable
	// doesn't exists pgm.SyncFromFollowedPGRewind will return an error and
	// fallback to pg_basebackup
	if tryPgrewind && p.usePgrewind(db) {
		connParams := p.getSUConnParams(db, followedDB)
		log.Infof("syncing using pg_rewind from followed db %q on keeper %q", followedDB.UID, followedDB.Spec.KeeperUID)
		if err := pgm.SyncFromFollowedPGRewind(connParams, p.pgSUPassword); err != nil {
			// log pg_rewind error and fallback to pg_basebackup
			log.Errorf("error syncing with pg_rewind: %v", err)
		} else {
			if err := pgm.WriteRecoveryConf(replConnParams, common.StolonName(db.UID)); err != nil {
				return fmt.Errorf("err: %v", err)
			}
			return nil
		}
	}

	if err := pgm.RemoveAll(); err != nil {
		return fmt.Errorf("failed to remove the postgres data dir: %v", err)
	}
	log.Debugf("syncing from followed db %q on keeper %q with connection params: %v", followedDB.UID, followedDB.Spec.KeeperUID, replConnParams)
	log.Infof("syncing from followed db %q on keeper %q", followedDB.UID, followedDB.Spec.KeeperUID)
	if err := pgm.SyncFromFollowed(replConnParams); err != nil {
		return fmt.Errorf("sync error: %v", err)
	}
	log.Info("sync succeeded")

	if err := pgm.WriteRecoveryConf(replConnParams, common.StolonName(db.UID)); err != nil {
		return fmt.Errorf("err: %v", err)
	}
	return nil
}

func (p *PostgresKeeper) isDifferentTimelineBranch(followedDB *cluster.DB, pgState *cluster.PostgresState) bool {
	if followedDB.Status.TimelineID < pgState.TimelineID {
		log.Infof("followed instance timeline %d < than our timeline %d", followedDB.Status.TimelineID, pgState.TimelineID)
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
		log.Infof("followed instance timeline %d forked at a different xlog pos %d then our timeline (timeline %d at xlog pos %d)", followedDB.Status.TimelineID, ftlh.SwitchPoint, pgState.TimelineID, tlh.SwitchPoint)
		return true
	}

	// followedDB.Status.TimelineID > pgState.TimelineID
	ftlh := followedDB.Status.TimelinesHistory.GetTimelineHistory(pgState.TimelineID)
	if ftlh != nil {
		if ftlh.SwitchPoint < pgState.XLogPos {
			log.Infof("followed instance timeline %d forked at xlog pos %d before our current state (timeline %d at xlog pos %d)", followedDB.Status.TimelineID, ftlh.SwitchPoint, pgState.TimelineID, pgState.XLogPos)
			return true
		}
	}
	return false
}

func (p *PostgresKeeper) postgresKeeperSM(pctx context.Context) {
	e := p.e
	pgm := p.pgm

	cd, _, err := e.GetClusterData()
	if err != nil {
		log.Errorf("error retrieving cluster data: %v", err)
		return
	}
	log.Debugf("cd: %s", spew.Sdump(cd))

	if cd == nil {
		log.Info("no cluster data available, waiting for it to appear")
		return
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		log.Errorf("unsupported clusterdata format version %d", cd.FormatVersion)
		return
	}
	if cd.Cluster != nil {
		p.sleepInterval = cd.Cluster.Spec.SleepInterval.Duration
		p.requestTimeout = cd.Cluster.Spec.RequestTimeout.Duration
	}

	k, ok := cd.Keepers[p.keeperLocalState.UID]
	if !ok {
		log.Info("our keeper data is not available, waiting for it to appear")
		return
	}
	// TODO(sgotti) Check that the Keeper.Status address:port has been updated

	db := cd.FindDB(k)
	if db == nil {
		log.Info("no db assigned")
		return
	}
	// TODO(sgotti) Check that the DB.Status address:port has been updated

	followersUIDs := db.Spec.Followers

	prevPGParameters := pgm.GetParameters()
	// create postgres parameteres
	pgParameters := p.createPGParameters(db)
	// update pgm postgres parameters
	pgm.SetParameters(pgParameters)

	pgm.SetIncludeConfig(db.Spec.IncludeConfig)

	dbls := p.dbLocalState
	if dbls.Initializing {
		// If we are here this means that the db initialization or
		// resync as failed so we have to clean up stale data
		log.Errorf("db failed to initialize or resync")
		// Clean up cluster db datadir
		if err = pgm.RemoveAll(); err != nil {
			log.Errorf("failed to remove the postgres data dir: %v", err)
			return
		}
		// Reset current db local state since it's not valid anymore
		p.localStateMutex.Lock()
		dbls.UID = ""
		dbls.Generation = cluster.NoGeneration
		dbls.Initializing = false
		p.localStateMutex.Unlock()
		if err = p.saveDBLocalState(); err != nil {
			log.Errorf("error: %v", err)
			return
		}
	}

	initialized, err := pgm.IsInitialized()
	if err != nil {
		log.Errorf("failed to detect if instance is initialized: %v", err)
		return
	}

	started := false
	if initialized {
		started, err = pgm.IsStarted()
		if err != nil {
			// log error getting instance state but go ahead.
			log.Infof("failed to retrieve instance status: %v", err)
		}
	}

	log.Debugf("started: %t", started)

	// if the db is initialized but there isn't a db local state then generate a new one
	if initialized && dbls.UID == "" {
		p.localStateMutex.Lock()
		dbls.UID = common.UID()
		dbls.Generation = cluster.NoGeneration
		dbls.Initializing = false
		p.localStateMutex.Unlock()
		if err = p.saveDBLocalState(); err != nil {
			log.Errorf("error: %v", err)
			return
		}
	}

	if dbls.UID != db.UID {
		log.Infof("current db UID %q different than cluster data db UID %q", dbls.UID, db.UID)
		switch db.Spec.InitMode {
		case cluster.DBInitModeNew:
			log.Info("initializing the database cluster")
			p.localStateMutex.Lock()
			dbls.UID = db.UID
			// Set a no generation since we aren't already converged.
			dbls.Generation = cluster.NoGeneration
			dbls.Initializing = true
			p.localStateMutex.Unlock()
			if err = p.saveDBLocalState(); err != nil {
				log.Errorf("error: %v", err)
				return
			}
			if started {
				if err = pgm.Stop(true); err != nil {
					log.Errorf("failed to stop pg instance: %v", err)
					return
				}
				started = false
			}
			if err = pgm.RemoveAll(); err != nil {
				log.Errorf("failed to remove the postgres data dir: %v", err)
				return
			}
			if err = pgm.Init(); err != nil {
				log.Errorf("failed to initialize postgres database cluster: %v", err)
				return
			}
			initialized = true
		case cluster.DBInitModeNone:
			log.Info("updating our db UID with the cluster data provided db UID")
			// If no init mode is required just replace our current
			// db uid with the required one.
			p.localStateMutex.Lock()
			dbls.UID = db.UID
			// Set a no generation since we aren't already converged.
			dbls.Generation = cluster.NoGeneration
			p.localStateMutex.Unlock()
			if err = p.saveDBLocalState(); err != nil {
				log.Errorf("error: %v", err)
				return
			}
		default:
			log.Errorf("unknown db init mode %q", db.Spec.InitMode)
			return
		}
	}

	var localRole common.Role
	var systemID string
	if !initialized {
		log.Info("database cluster not initialized")
		localRole = common.RoleUndefined
	} else {
		localRole, err = pgm.GetRole()
		if err != nil {
			log.Infof("error retrieving current pg role: %v", err)
			return
		}
		systemID, err = p.pgm.GetSystemdID()
		if err != nil {
			log.Errorf("error retrieving systemd ID: %v", err)
			return
		}
	}

	targetRole := db.Spec.Role
	log.Debugf("target role: %s", targetRole)

	switch targetRole {
	case common.RoleMaster:
		// We are the elected master
		log.Info("our db requested role is master")
		if localRole == common.RoleUndefined {
			log.Errorf("database cluster not initialized but requested role is master. This shouldn't happen!")
			return
		}
		if !started {
			if err = pgm.Start(); err != nil {
				log.Errorf("failed to start postgres: %v", err)
				return
			}
			started = true
		}

		if localRole == common.RoleStandby {
			log.Info("promoting to master")
			if err = pgm.Promote(); err != nil {
				log.Errorf("err: %v", err)
				return
			}
		} else {
			log.Info("already master")
		}

		var replSlots []string
		replSlots, err = pgm.GetReplicatinSlots()
		log.Debugf("replSlots: %v", replSlots)
		if err != nil {
			log.Errorf("err: %v", err)
			return
		}
		// Drop replication slots
		for _, slotName := range replSlots {
			if !common.IsStolonName(slotName) {
				continue
			}
			if !util.StringInSlice(followersUIDs, common.NameFromStolonName(slotName)) {
				log.Infof("dropping replication slot %q since db %q not marked as follower", slotName, common.NameFromStolonName(slotName))
				if err = pgm.DropReplicationSlot(slotName); err != nil {
					log.Errorf("err: %v", err)
				}
			}
		}
		// Create replication slots
		for _, followerUID := range followersUIDs {
			if followerUID == dbls.UID {
				continue
			}
			replSlot := common.StolonName(followerUID)
			if !util.StringInSlice(replSlots, replSlot) {
				if err = pgm.CreateReplicationSlot(replSlot); err != nil {
					log.Infof("creating replication slot %q for db %q", replSlot, followerUID)
					log.Errorf("err: %v", err)
				}
			}
		}
	case common.RoleStandby:
		// We are a standby
		followedUID := db.Spec.FollowConfig.DBUID
		log.Infof("our db requested role is standby following db %q", followedUID)
		followedDB, ok := cd.DBs[followedUID]
		if !ok {
			log.Errorf("no db data for keeper %q", followedUID)
			return
		}
		switch localRole {
		case common.RoleMaster:
			if systemID == followedDB.Status.SystemID {
				// There can be the possibility that this
				// database is on the same branch of the
				// current followed instance.
				// So we try to put it in recovery and then
				// check if it's on the same branch or force a
				// resync
				replConnParams := p.getReplConnParams(db, followedDB)
				if err = pgm.WriteRecoveryConf(replConnParams, common.StolonName(db.UID)); err != nil {
					log.Errorf("err: %v", err)
					return
				}
				if !started {
					if err = pgm.Start(); err != nil {
						log.Errorf("err: %v", err)
						return
					}
					started = true
				} else {
					if err = pgm.Restart(true); err != nil {
						log.Errorf("err: %v", err)
						return
					}
				}

				// Check timeline history
				// We need to update our pgState to avoid dealing with
				// an old pgState not reflecting the real state
				var pgState *cluster.PostgresState
				pgState, err = p.GetPGState(pctx)
				if err != nil {
					log.Errorf("cannot get current pgstate: %v", err)
					return
				}

				if p.isDifferentTimelineBranch(followedDB, pgState) {
					if err = p.resync(db, followedDB, true, started); err != nil {
						log.Errorf("failed to resync from followed instance: %v", err)
						return
					}
					if err = pgm.Start(); err != nil {
						log.Errorf("err: %v", err)
						return
					}
					started = true
				}
			} else {
				if err = p.resync(db, followedDB, false, started); err != nil {
					log.Errorf("failed to resync from followed instance: %v", err)
					return
				}
				if err = pgm.Start(); err != nil {
					log.Errorf("err: %v", err)
					return
				}
				started = true
			}
		case common.RoleStandby:
			log.Info("already standby")
			if !started {
				replConnParams := p.getReplConnParams(db, followedDB)
				if err = pgm.WriteRecoveryConf(replConnParams, common.StolonName(db.UID)); err != nil {
					log.Errorf("err: %v", err)
					return
				}
				if err = pgm.Start(); err != nil {
					log.Errorf("failed to start postgres: %v", err)
					return
				}
				started = true
			}

			// Check that we can sync with followed instance

			// We need to update our pgState to avoid dealing with
			// an old pgState not reflecting the real state
			var pgState *cluster.PostgresState
			pgState, err = p.GetPGState(pctx)
			if err != nil {
				log.Errorf("cannot get current pgstate: %v", err)
				return
			}
			needsResync := false
			// If the db has a different systemdID then a resync is needed
			if systemID != followedDB.Status.SystemID {
				needsResync = true
				// Check timeline history
			} else if p.isDifferentTimelineBranch(followedDB, pgState) {
				needsResync = true
			}
			if needsResync {
				if err = p.resync(db, followedDB, true, started); err != nil {
					log.Errorf("failed to full resync from followed instance: %v", err)
					return
				}
				if err = pgm.Start(); err != nil {
					log.Errorf("err: %v", err)
					return
				}
				started = true
			}

			// TODO(sgotti) Check that the followed instance has all the needed WAL segments

			// Update our primary_conninfo if replConnString changed
			var curReplConnParams postgresql.ConnParams

			curReplConnParams, err = pgm.GetPrimaryConninfo()
			if err != nil {
				log.Errorf("err: %v", err)
				return
			}
			log.Debugf(spew.Sprintf("curReplConnParams: %v", curReplConnParams))

			newReplConnParams := p.getReplConnParams(db, followedDB)
			log.Debugf(spew.Sprintf("newReplConnParams: %v", newReplConnParams))

			if !curReplConnParams.Equals(newReplConnParams) {
				log.Info("connection parameters changed. Reconfiguring...")
				log.Infof("following %q with connection parameters %v", followedUID, newReplConnParams)
				if err = pgm.WriteRecoveryConf(newReplConnParams, common.StolonName(db.UID)); err != nil {
					log.Errorf("err: %v", err)
					return
				}
				if err = pgm.Restart(true); err != nil {
					log.Errorf("err: %v", err)
					return
				}
			}
		case common.RoleUndefined:
			if err = p.resync(db, followedDB, true, started); err != nil {
				log.Errorf("failed to full resync from followed instance: %v", err)
				return
			}
			if err = pgm.Start(); err != nil {
				log.Errorf("err: %v", err)
				return
			}
			started = true
		}
	case common.RoleUndefined:
		log.Info("our db requested role is none")
		return
	}

	// update pg parameters
	pgParameters = p.createPGParameters(db)

	// Log synchronous replication changes
	prevSyncStandbyNames := prevPGParameters["synchronous_standby_names"]
	syncStandbyNames := pgParameters["synchronous_standby_names"]
	if db.Spec.SynchronousReplication {
		if prevSyncStandbyNames != syncStandbyNames {
			log.Infof("needed synchronous_standby_names changed from %q to %q", prevSyncStandbyNames, syncStandbyNames)
		}
	} else {
		if prevSyncStandbyNames != "" {
			log.Infof("sync replication disabled, removing current synchronous_standby_names %q", prevSyncStandbyNames)
		}
	}

	if !pgParameters.Equals(prevPGParameters) {
		log.Info("postgres parameters changed, reloading postgres instance")
		pgm.SetParameters(pgParameters)
		if err := pgm.Reload(); err != nil {
			log.Errorf("failed to reload postgres instance: %v", err)
		}
	} else {
		// for tests
		log.Info("postgres parameters not changed")
	}

	// If we are here, then all went well and we can update the db generation and save it locally
	p.localStateMutex.Lock()
	dbls.Generation = db.Generation
	dbls.Initializing = false
	p.localStateMutex.Unlock()
	if err := p.saveDBLocalState(); err != nil {
		log.Errorf("err: %v", err)
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
	return ioutil.WriteFile(p.keeperLocalStateFilePath(), sj, 0600)
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
	return ioutil.WriteFile(p.dbLocalStateFilePath(), sj, 0600)
}

func sigHandler(sigs chan os.Signal, stop chan bool) {
	s := <-sigs
	log.Debugf("got signal: %s", s)
	close(stop)
}

func main() {
	flagutil.SetFlagsFromEnv(cmdKeeper.PersistentFlags(), "STKEEPER")

	cmdKeeper.Execute()
}

func keeper(cmd *cobra.Command, args []string) {
	var err error
	capnslog.SetGlobalLogLevel(capnslog.INFO)
	if cfg.debug {
		capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	}
	if cfg.dataDir == "" {
		log.Fatalf("data dir required")
	}

	if cfg.clusterName == "" {
		log.Fatalf("cluster name required")
	}
	if cfg.storeBackend == "" {
		log.Fatalf("store backend type required")
	}

	if err = os.MkdirAll(cfg.dataDir, 0700); err != nil {
		log.Fatalf("error: %v", err)
	}

	if cfg.pgReplUsername == "" {
		log.Fatalf("--pg-repl-username is required")
	}

	if cfg.pgReplPassword == "" && cfg.pgReplPasswordFile == "" {
		log.Fatalf("one of --pg-repl-password or --pg-repl-passwordfile is required")
	}
	if cfg.pgReplPassword != "" && cfg.pgReplPasswordFile != "" {
		log.Fatalf("only one of --pg-repl-password or --pg-repl-passwordfile must be provided")
	}
	if cfg.pgSUPassword == "" && cfg.pgSUPasswordFile == "" {
		log.Fatalf("one of --pg-su-password or --pg-su-passwordfile is required")
	}
	if cfg.pgSUPassword != "" && cfg.pgSUPasswordFile != "" {
		log.Fatalf("only one of --pg-su-password or --pg-su-passwordfile must be provided")
	}

	if cfg.pgSUUsername == cfg.pgReplUsername {
		log.Warning("superuser name and replication user name are the same. Different users are suggested.")
		if cfg.pgSUPassword != cfg.pgReplPassword {
			log.Fatalf("provided superuser name and replication user name are the same but provided passwords are different")
		}
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

	// Take an exclusive lock on dataDir
	_, err = lock.TryExclusiveLock(cfg.dataDir, lock.Dir)
	if err != nil {
		log.Fatalf("cannot take exclusive lock on data dir %q: %v", cfg.dataDir, err)
	}

	if cfg.id != "" {
		if !pg.IsValidReplSlotName(cfg.id) {
			log.Fatalf("keeper id %q not valid. It can contain only lower-case letters, numbers and the underscore character", cfg.id)
		}
	}

	if kubernetes.OnKubernetes() {
		log.Info("running under kubernetes.")
	}

	stop := make(chan bool, 0)
	end := make(chan error, 0)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill)
	go sigHandler(sigs, stop)

	p, err := NewPostgresKeeper(&cfg, stop, end)
	if err != nil {
		log.Fatalf("cannot create keeper: %v", err)
	}
	go p.Start()

	<-end
}
