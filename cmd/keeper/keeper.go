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
	etcdm "github.com/sorintlab/stolon/pkg/etcd"
	"github.com/sorintlab/stolon/pkg/flagutil"
	"github.com/sorintlab/stolon/pkg/kubernetes"
	"github.com/sorintlab/stolon/pkg/postgresql"
	pg "github.com/sorintlab/stolon/pkg/postgresql"
	"github.com/sorintlab/stolon/pkg/util"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/pkg/capnslog"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/rkt/pkg/lock"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/spf13/cobra"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
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

type config struct {
	id              string
	etcdEndpoints   string
	dataDir         string
	clusterName     string
	listenAddress   string
	port            string
	pgListenAddress string
	pgPort          string
	pgBinPath       string
	pgConfDir       string
	debug           bool
}

var cfg config

func init() {
	cmdKeeper.PersistentFlags().StringVar(&cfg.id, "id", "", "keeper id (must be unique in the cluster and can contain only lower-case letters, numbers and the underscore character). If not provided a random id will be generated.")
	cmdKeeper.PersistentFlags().StringVar(&cfg.etcdEndpoints, "etcd-endpoints", common.DefaultEtcdEndpoints, "a comma-delimited list of etcd endpoints")
	cmdKeeper.PersistentFlags().StringVar(&cfg.dataDir, "data-dir", "", "data directory")
	cmdKeeper.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdKeeper.PersistentFlags().StringVar(&cfg.listenAddress, "listen-address", "localhost", "keeper listening address")
	cmdKeeper.PersistentFlags().StringVar(&cfg.port, "port", "5431", "keeper listening port")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgListenAddress, "pg-listen-address", "localhost", "postgresql instance listening address")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgPort, "pg-port", "5432", "postgresql instance listening port")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgBinPath, "pg-bin-path", "", "absolute path to postgresql binaries. If empty they will be searched in the current PATH")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgConfDir, "pg-conf-dir", "", "absolute path to user provided postgres configuration. If empty a default dir under $dataDir/postgres/conf.d will be used")
	cmdKeeper.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
}

var defaultPGParameters = pg.Parameters{
	"unix_socket_directories": "/tmp",
	"wal_level":               "hot_standby",
	"wal_keep_segments":       "8",
	"hot_standby":             "on",
}

func (p *PostgresKeeper) getReplConnString(keeperState *cluster.KeeperState) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s?sslmode=disable&application_name=%s", p.clusterConfig.PGReplUser, p.clusterConfig.PGReplPassword, keeperState.PGListenAddress, keeperState.PGPort, p.id)
}

func (p *PostgresKeeper) getOurConnString() string {
	return fmt.Sprintf("postgres://%s:%s/postgres?sslmode=disable", "localhost", p.pgPort)
}

func (p *PostgresKeeper) getOurReplConnString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s?sslmode=disable", p.clusterConfig.PGReplUser, p.clusterConfig.PGReplPassword, p.pgListenAddress, p.pgPort)
}

func (p *PostgresKeeper) createPGParameters(followersIDs []string) pg.Parameters {
	pgParameters := p.clusterConfig.PGParameters

	// Merge default PGParameters
	for k, v := range defaultPGParameters.Copy() {
		pgParameters[k] = v
	}

	pgParameters["listen_addresses"] = fmt.Sprintf("127.0.0.1,%s", cfg.pgListenAddress)
	pgParameters["port"] = cfg.pgPort
	pgParameters["max_replication_slots"] = strconv.FormatUint(uint64(p.clusterConfig.MaxStandbysPerSender), 10)
	// Add some more wal senders, since also the keeper will use them
	pgParameters["max_wal_senders"] = strconv.FormatUint(uint64(p.clusterConfig.MaxStandbysPerSender+2), 10)

	// Setup synchronous replication
	if p.clusterConfig.SynchronousReplication {
		pgParameters["synchronous_standby_names"] = strings.Join(followersIDs, ",")
	} else {
		pgParameters["synchronous_standby_names"] = ""
	}

	return pgParameters
}

type PostgresKeeper struct {
	id      string
	dataDir string
	e       *etcdm.EtcdManager
	pgm     *postgresql.Manager
	stop    chan bool
	end     chan error

	listenAddress   string
	port            string
	pgListenAddress string
	pgPort          string

	clusterConfig *cluster.Config

	cvVersionMutex sync.Mutex
	cvVersion      int

	pgStateMutex sync.Mutex
	lastPGState  *cluster.PostgresState
}

func NewPostgresKeeper(id string, cfg config, stop chan bool, end chan error) (*PostgresKeeper, error) {
	etcdPath := filepath.Join(common.EtcdBasePath, cfg.clusterName)
	e, err := etcdm.NewEtcdManager(cfg.etcdEndpoints, etcdPath, common.DefaultEtcdRequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("cannot create etcd manager: %v", err)
	}

	cd, _, err := e.GetClusterData()
	if err != nil {
		return nil, fmt.Errorf("error retrieving cluster data: %v", err)
	}

	var cv *cluster.ClusterView
	if cd == nil {
		cv = cluster.NewClusterView()
	} else {
		cv = cd.ClusterView
	}
	log.Debugf(spew.Sprintf("clusterView: %#v", cv))

	clusterConfig := cv.Config.ToConfig()
	log.Debugf(spew.Sprintf("clusterConfig: %#v", clusterConfig))

	p := &PostgresKeeper{id: id,
		dataDir:         cfg.dataDir,
		e:               e,
		listenAddress:   cfg.listenAddress,
		port:            cfg.port,
		pgListenAddress: cfg.pgListenAddress,
		pgPort:          cfg.pgPort,
		clusterConfig:   clusterConfig,
		stop:            stop,
		end:             end,
	}

	followersIDs := cv.GetFollowersIDs(p.id)
	pgParameters := p.createPGParameters(followersIDs)
	pgm, err := postgresql.NewManager(id, cfg.pgBinPath, cfg.dataDir, cfg.pgConfDir, pgParameters, p.getOurConnString(), p.getOurReplConnString(), clusterConfig.PGReplUser, clusterConfig.PGReplPassword, clusterConfig.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("cannot create postgres manager: %v", err)
	}
	p.pgm = pgm
	return p, nil
}

func (p *PostgresKeeper) publish() error {
	if kubernetes.OnKubernetes() {
		log.Infof("running under kubernetes. Not publishing ourself to etcd")
		return nil
	}
	discoveryInfo := &cluster.KeeperDiscoveryInfo{
		ListenAddress: p.listenAddress,
		Port:          p.port,
	}
	log.Debugf(spew.Sprintf("discoveryInfo: %#v", discoveryInfo))

	if _, err := p.e.SetKeeperDiscoveryInfo(p.id, discoveryInfo); err != nil {
		return err
	}
	return nil
}

func (p *PostgresKeeper) saveCVVersion(version int) error {
	p.cvVersionMutex.Lock()
	defer p.cvVersionMutex.Unlock()
	p.cvVersion = version
	return ioutil.WriteFile(filepath.Join(p.dataDir, "cvversion"), []byte(strconv.Itoa(version)), 0600)
}

func (p *PostgresKeeper) loadCVVersion() error {
	p.cvVersionMutex.Lock()
	defer p.cvVersionMutex.Unlock()
	cvVersion, err := ioutil.ReadFile(filepath.Join(p.dataDir, "cvversion"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	p.cvVersion, err = strconv.Atoi(string(cvVersion))
	return err
}

func (p *PostgresKeeper) infoHandler(w http.ResponseWriter, req *http.Request) {
	p.cvVersionMutex.Lock()
	defer p.cvVersionMutex.Unlock()

	keeperInfo := cluster.KeeperInfo{
		ID:                 p.id,
		ClusterViewVersion: p.cvVersion,
		ListenAddress:      p.listenAddress,
		Port:               p.port,
		PGListenAddress:    p.pgListenAddress,
		PGPort:             p.pgPort,
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
	pgState := &cluster.PostgresState{}

	defer func() {
		p.lastPGState = pgState
		p.pgStateMutex.Unlock()
	}()

	initialized, err := p.pgm.IsInitialized()
	if err != nil {
		pgState = nil
		return
	}
	if !initialized {
		pgState.Initialized = false
	} else {
		var err error
		ctx, cancel := context.WithTimeout(pctx, p.clusterConfig.RequestTimeout)
		pgState, err = pg.GetPGState(ctx, p.getOurReplConnString())
		cancel()
		if err != nil {
			log.Errorf("error getting pg state: %v", err)
			pgState = nil
			return
		}
		pgState.Initialized = true

		// if timeline <= 1 then no timeline history file exists.
		pgState.TimelinesHistory = cluster.PostgresTimeLinesHistory{}
		if pgState.TimelineID > 1 {
			ctx, cancel = context.WithTimeout(pctx, p.clusterConfig.RequestTimeout)
			tlsh, err := pg.GetTimelinesHistory(ctx, pgState.TimelineID, p.getOurReplConnString())
			cancel()
			if err != nil {
				log.Errorf("error getting timeline history: %v", err)
				pgState = nil
				return
			}
			pgState.TimelinesHistory = tlsh
		}
	}
}

func (p *PostgresKeeper) getLastPGState() *cluster.PostgresState {
	p.pgStateMutex.Lock()
	pgState := p.lastPGState.Copy()
	p.pgStateMutex.Unlock()
	return pgState
}

func (p *PostgresKeeper) Start() {
	endSMCh := make(chan struct{})
	endPgStatecheckerCh := make(chan struct{})
	endApiCh := make(chan error)

	err := p.loadCVVersion()
	if err != nil {
		p.end <- err
		return
	}

	p.pgm.Stop(true)

	http.HandleFunc("/info", p.infoHandler)
	http.HandleFunc("/pgstate", p.pgStateHandler)
	go func() {
		endApiCh <- http.ListenAndServe(fmt.Sprintf("%s:%s", p.listenAddress, p.port), nil)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	smTimerCh := time.NewTimer(0).C
	updatePGStateTimerCh := time.NewTimer(0).C
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
			smTimerCh = time.NewTimer(p.clusterConfig.SleepInterval).C
		case <-updatePGStateTimerCh:
			go func() {
				p.updatePGState(ctx)
				endPgStatecheckerCh <- struct{}{}
			}()
		case <-endPgStatecheckerCh:
			updatePGStateTimerCh = time.NewTimer(p.clusterConfig.SleepInterval).C
		case err := <-endApiCh:
			if err != nil {
				log.Fatal("ListenAndServe: ", err)
			}
			close(p.stop)
		}
	}
}

func (p *PostgresKeeper) fullResync(master *cluster.KeeperState, initialized, started bool) error {
	pgm := p.pgm
	if initialized && started {
		err := pgm.Stop(true)
		if err != nil {
			return fmt.Errorf("failed to stop pg instance: %v", err)
		}
	}
	err := pgm.RemoveAll()
	if err != nil {
		return fmt.Errorf("failed to remove the postgres data dir: %v", err)
	}
	replConnString := p.getReplConnString(master)
	log.Infof("syncing from master %q with connection url %q", master.ID, replConnString)
	err = pgm.SyncFromMaster(replConnString)
	if err != nil {
		return fmt.Errorf("error: %v", err)
	}
	log.Infof("sync from master %q successfully finished", master.ID)

	err = pgm.BecomeStandby(replConnString)
	if err != nil {
		return fmt.Errorf("err: %v", err)
	}

	err = pgm.Start()
	if err != nil {
		return fmt.Errorf("failed to start postgres: %v", err)
	}
	return nil
}

func (p *PostgresKeeper) isDifferentTimelineBranch(mPGState *cluster.PostgresState, pgState *cluster.PostgresState) bool {
	if mPGState.TimelineID < pgState.TimelineID {
		return true
	}
	if mPGState.TimelineID == pgState.TimelineID {
		return false
	}

	// mPGState.TimelineID > pgState.TimelineID
	tlh := mPGState.TimelinesHistory.GetTimelineHistory(pgState.TimelineID)
	if tlh != nil {
		if tlh.SwitchPoint < pgState.XLogPos {
			log.Infof("master timeline %d forked at xlog pos %d before our current state (timeline %d at xlog pos %d)", mPGState.TimelineID, tlh.SwitchPoint, pgState.TimelineID, pgState.XLogPos)
			return true
		}
	}
	return false
}

func (p *PostgresKeeper) postgresKeeperSM(pctx context.Context) {
	e := p.e
	pgm := p.pgm

	cv, _, err := e.GetClusterView()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("clusterView: %#v", cv))

	if cv == nil {
		log.Infof("no clusterview available, waiting for it to appear")
		return
	}

	followersIDs := cv.GetFollowersIDs(p.id)

	// Update cluster config
	clusterConfig := cv.Config.ToConfig()
	log.Debugf(spew.Sprintf("clusterConfig: %#v", clusterConfig))
	// This shouldn't need a lock
	p.clusterConfig = clusterConfig

	prevPGParameters := pgm.GetParameters()
	// create postgres parameteres
	pgParameters := p.createPGParameters(followersIDs)
	// update pgm postgres parameters
	pgm.SetParameters(pgParameters)

	keepersState, _, err := e.GetKeepersState()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	if keepersState == nil {
		keepersState = cluster.KeepersState{}
	}
	log.Debugf(spew.Sprintf("keepersState: %#v", keepersState))

	keeper := keepersState[p.id]
	log.Debugf(spew.Sprintf("keeperState: %#v", keeper))

	initialized, err := pgm.IsInitialized()
	if err != nil {
		log.Errorf("failed to detect if instance is initialized: %v", err)
		return
	}

	if len(cv.KeepersRole) == 0 {
		if !initialized {
			log.Infof("Initializing database")
			err = pgm.Init()
			if err != nil {
				log.Errorf("failed to initialized postgres instance: %v", err)
				return
			}
			initialized = true
		}
	}

	started := false

	if initialized {
		started, err = pgm.IsStarted()
		if err != nil {
			log.Errorf("failed to retrieve instance status: %v", err)
		} else if !started {
			err = pgm.Start()
			if err != nil {
				log.Errorf("failed to start postgres: %v", err)
			} else {
				started = true
			}
		}
	}

	if cv != nil {
		if !started && p.id == cv.Master {
			// If the clusterView says we are master but we cannot get
			// instance status or start then stop here, if we are standby then we can
			// recover
			return
		}
	}

	role, err := pgm.GetRole()
	if err != nil {
		log.Infof("error retrieving current pg role: %v", err)
		return
	}
	isMaster := false
	if role == common.MasterRole {
		log.Infof("current pg state: master")
		isMaster = true
	} else {
		log.Infof("current pg state: standby")
	}

	// publish ourself for discovery
	if err := p.publish(); err != nil {
		log.Errorf("failed to publish ourself to the cluster: %v", err)
		return
	}

	if cv == nil {
		return
	}

	// cv != nil

	masterID := cv.Master
	log.Debugf("masterID: %q", masterID)

	master := keepersState[masterID]
	log.Debugf(spew.Sprintf("masterState: %#v", master))

	keeperRole, ok := cv.KeepersRole[p.id]
	if !ok {
		log.Infof("our keeper requested role is not available")
		return
	}
	if keeperRole.Follow == "" {
		log.Infof("our cluster requested state is master")
		if role != common.MasterRole {
			log.Infof("promoting to master")
			err := pgm.Promote()
			if err != nil {
				log.Errorf("err: %v", err)
				return
			}
		} else {
			log.Infof("already master")

			replSlots := []string{}
			replSlots, err = pgm.GetReplicatinSlots()
			if err != nil {
				log.Errorf("err: %v", err)
				return
			}
			// Create replication slots
			for _, slotName := range replSlots {
				if !util.StringInSlice(followersIDs, slotName) {
					log.Infof("dropping replication slot for keeper %q not marked as follower", slotName)
					err := pgm.DropReplicationSlot(slotName)
					if err != nil {
						log.Errorf("err: %v", err)
					}
				}
			}

			for _, followerID := range followersIDs {
				if followerID == p.id {
					continue
				}
				if !util.StringInSlice(replSlots, followerID) {
					err := pgm.CreateReplicationSlot(followerID)
					if err != nil {
						log.Errorf("err: %v", err)
					}
				}
			}

		}
	} else {
		log.Infof("our cluster requested state is standby following %q", keeperRole.Follow)
		if isMaster {
			if err := p.fullResync(master, initialized, started); err != nil {
				log.Errorf("failed to full resync from master: %v", err)
				return
			}
		} else {
			log.Infof("already standby")
			curConnParams, err := pgm.GetPrimaryConninfo()
			if err != nil {
				log.Errorf("err: %v", err)
				return
			}
			log.Debugf(spew.Sprintf("curConnParams: %v", curConnParams))

			replConnString := p.getReplConnString(master)
			newConnParams, err := pg.URLToConnParams(replConnString)
			if err != nil {
				log.Errorf("cannot get conn params: %v", err)
				return
			}
			log.Debugf(spew.Sprintf("newConnParams: %v", newConnParams))

			// Check that we can sync with master

			// Check timeline history
			// We need to update our pgState to avoid dealing with
			// an old pgState not reflecting the real state
			p.updatePGState(pctx)
			pgState := p.getLastPGState()
			if pgState == nil {
				log.Errorf("our pgstate is unknown: %v", err)
				return
			}
			mPGState := master.PGState
			if p.isDifferentTimelineBranch(mPGState, pgState) {
				if err := p.fullResync(master, initialized, started); err != nil {
					log.Errorf("failed to full resync from master: %v", err)
					return
				}
			}

			// TODO(sgotti) Check that the master has all the needed WAL segments

			// Update our primary_conninfo if replConnString changed
			if !curConnParams.Equals(newConnParams) {
				log.Infof("master connection parameters changed. Reconfiguring...")
				log.Infof("following %s with connection url %s", keeperRole.Follow, replConnString)
				err = pgm.BecomeStandby(replConnString)
				if err != nil {
					log.Errorf("err: %v", err)
					return
				}
				err = pgm.Restart(true)
				if err != nil {
					log.Errorf("err: %v", err)
					return
				}
			}
		}
	}

	// Log synchronous replication changes
	prevSyncStandbyNames := prevPGParameters["synchronous_standby_names"]
	syncStandbyNames := pgParameters["synchronous_standby_names"]
	if p.clusterConfig.SynchronousReplication {
		if prevSyncStandbyNames != syncStandbyNames {
			log.Infof("needed synchronous_standby_names changed from %q to %q", prevSyncStandbyNames, syncStandbyNames)
		}
	} else {
		if prevSyncStandbyNames != "" {
			log.Infof("sync replication disabled, removing current synchronous_standby_names %q", prevSyncStandbyNames)
		}
	}

	if !pgParameters.Equals(prevPGParameters) {
		log.Infof("postgres parameters changed, reloading postgres instance")
		pgm.SetParameters(pgParameters)
		if err := pgm.Reload(); err != nil {
			log.Errorf("failed to reload postgres instance: %v", err)
		}
	} else {
		// for tests
		log.Debugf("postgres parameters not changed")
	}
	if err := p.saveCVVersion(cv.Version); err != nil {
		log.Errorf("err: %v", err)
		return
	}
}

func getIDFilePath(conf config) string {
	return filepath.Join(conf.dataDir, "pgid")
}
func getIDFromFile(conf config) (string, error) {
	id, err := ioutil.ReadFile(getIDFilePath(conf))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(id), nil
}

func saveIDToFile(conf config, id string) error {
	return ioutil.WriteFile(getIDFilePath(conf), []byte(id), 0600)
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

	if err := os.MkdirAll(cfg.dataDir, 0700); err != nil {
		log.Fatalf("error: %v", err)
	}

	if cfg.pgConfDir != "" {
		if !filepath.IsAbs(cfg.pgConfDir) {
			log.Fatalf("pg-conf-dir must be an absolute path")
		}
		fi, err := os.Stat(cfg.pgConfDir)
		if err != nil {
			log.Fatalf("cannot stat pg-conf-dir: %v", err)
		}
		if !fi.IsDir() {
			log.Fatalf("pg-conf-dir is not a directory")
		}
	}

	// Take an exclusive lock on dataDir
	_, err := lock.TryExclusiveLock(cfg.dataDir, lock.Dir)
	if err != nil {
		log.Fatalf("cannot take exclusive lock on data dir %q: %v", cfg.dataDir, err)
	}

	if cfg.id != "" {
		if !pg.IsValidReplSlotName(cfg.id) {
			log.Fatalf("keeper id %q not valid. It can contain only lower-case letters, numbers and the underscore character", cfg.id)
		}
	}
	id, err := getIDFromFile(cfg)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	if id != "" && cfg.id != "" && id != cfg.id {
		log.Fatalf("saved id: %q differs from configuration id: %q", id, cfg.id)
	}
	if id == "" {
		id = cfg.id
		if cfg.id == "" {
			// Generate a new id if conf.Name is empty
			if id == "" {
				u := uuid.NewV4()
				id = fmt.Sprintf("%x", u[:4])
			}
			log.Infof("generated id: %s", id)
		}
		if err := saveIDToFile(cfg, id); err != nil {
			log.Fatalf("error: %v", err)
		}
	}

	log.Infof("id: %s", id)

	stop := make(chan bool, 0)
	end := make(chan error, 0)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill)
	go sigHandler(sigs, stop)

	p, err := NewPostgresKeeper(id, cfg, stop, end)
	if err != nil {
		log.Fatalf("cannot create keeper: %v", err)
	}
	go p.Start()

	<-end
}
