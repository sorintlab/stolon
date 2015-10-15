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
	debug           bool
}

var cfg config

func init() {
	cmdKeeper.PersistentFlags().StringVar(&cfg.id, "id", "", "keeper id (must be unique in the cluster)")
	cmdKeeper.PersistentFlags().StringVar(&cfg.etcdEndpoints, "etcd-endpoints", "http://127.0.0.1:4001,http://127.0.0.1:2379", "a comma-delimited list of etcd endpoints")
	cmdKeeper.PersistentFlags().StringVar(&cfg.dataDir, "data-dir", "", "data directory")
	cmdKeeper.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdKeeper.PersistentFlags().StringVar(&cfg.listenAddress, "listen-address", "localhost", "keeper listening address")
	cmdKeeper.PersistentFlags().StringVar(&cfg.port, "port", "5431", "keeper listening port")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgListenAddress, "pg-listen-address", "localhost", "postgresql instance listening address")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgPort, "pg-port", "5432", "postgresql instance listening port")
	cmdKeeper.PersistentFlags().StringVar(&cfg.pgBinPath, "pg-bin-path", "", "path to postgresql binaries. If empty they will be searched in the current PATH")
	cmdKeeper.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
}

func (p *PostgresKeeper) getReplConnString(memberState *cluster.MemberState) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s?sslmode=disable", p.clusterConfig.PGReplUser, p.clusterConfig.PGReplPassword, memberState.PGListenAddress, memberState.PGPort)
}

func (p *PostgresKeeper) getOurReplConnString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s?sslmode=disable", p.clusterConfig.PGReplUser, p.clusterConfig.PGReplPassword, p.pgListenAddress, p.pgPort)
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

	cvMutex   sync.Mutex
	cvVersion int
}

func NewPostgresKeeper(id string, cfg config, stop chan bool, end chan error) (*PostgresKeeper, error) {
	etcdPath := filepath.Join(common.EtcdBasePath, cfg.clusterName)
	e, err := etcdm.NewEtcdManager(cfg.etcdEndpoints, etcdPath, common.DefaultEtcdRequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("cannot create etcd manager: %v", err)
	}

	clusterConfig, _, err := e.GetClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster config: %v", err)
	}
	log.Debugf(spew.Sprintf("clusterConfig: %+v", clusterConfig))

	pgm, err := postgresql.NewManager(id, cfg.pgBinPath, cfg.dataDir, cfg.pgListenAddress, cfg.pgPort, clusterConfig.PGReplUser, clusterConfig.PGReplPassword, clusterConfig.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("cannot create postgres manager: %v", err)
	}

	return &PostgresKeeper{id: id,
		dataDir:         cfg.dataDir,
		e:               e,
		pgm:             pgm,
		listenAddress:   cfg.listenAddress,
		port:            cfg.port,
		pgListenAddress: cfg.pgListenAddress,
		pgPort:          cfg.pgPort,
		clusterConfig:   clusterConfig,
		stop:            stop,
		end:             end}, nil
}

func (p *PostgresKeeper) publish() error {
	if kubernetes.OnKubernetes() {
		log.Infof("running under kubernetes. Not publishing ourself to etcd")
		return nil
	}
	discoveryInfo := &cluster.MemberDiscoveryInfo{
		Host: p.listenAddress,
		Port: p.port,
	}
	log.Debugf(spew.Sprintf("discoveryInfo: %#v", discoveryInfo))

	if _, err := p.e.SetMemberDiscoveryInfo(p.id, discoveryInfo); err != nil {
		return err
	}
	return nil
}

func (p *PostgresKeeper) saveCVVersion(version int) error {
	p.cvMutex.Lock()
	defer p.cvMutex.Unlock()
	p.cvVersion = version
	return ioutil.WriteFile(filepath.Join(p.dataDir, "cvversion"), []byte(strconv.Itoa(version)), 0600)
}

func (p *PostgresKeeper) loadCVVersion() error {
	p.cvMutex.Lock()
	defer p.cvMutex.Unlock()
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
	p.cvMutex.Lock()
	defer p.cvMutex.Unlock()

	memberInfo := cluster.MemberInfo{
		ID:                 p.id,
		ClusterViewVersion: p.cvVersion,
		Host:               p.listenAddress,
		Port:               p.port,
		PGListenAddress:    p.pgListenAddress,
		PGPort:             p.pgPort,
	}

	if err := json.NewEncoder(w).Encode(&memberInfo); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (p *PostgresKeeper) pgStateHandler(w http.ResponseWriter, req *http.Request) {
	pgState := &cluster.PostgresState{}
	p.cvMutex.Lock()
	defer p.cvMutex.Unlock()

	initialized, err := p.pgm.IsInitialized()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !initialized {
		pgState.Initialized = false
	} else {
		var err error
		ctx, cancel := context.WithTimeout(context.Background(), p.clusterConfig.RequestTimeout)
		pgState, err = pg.GetPGState(ctx, p.getOurReplConnString())
		cancel()
		if err != nil {
			log.Errorf("error getting pg state: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		pgState.Initialized = true

		// if timeline <= 1 then no timeline history file exists.
		pgState.TimelinesHistory = cluster.PostgresTimeLinesHistory{}
		if pgState.TimelineID > 1 {
			ctx, cancel = context.WithTimeout(context.Background(), p.clusterConfig.RequestTimeout)
			tlsh, err := pg.GetTimelinesHistory(ctx, pgState.TimelineID, p.getOurReplConnString())
			cancel()
			if err != nil {
				log.Errorf("error getting timeline history: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			pgState.TimelinesHistory = tlsh
		}
	}
	if err := json.NewEncoder(w).Encode(&pgState); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (p *PostgresKeeper) Start() {
	endCh := make(chan struct{})
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
	timerCh := time.NewTimer(0).C
	for true {
		select {
		case <-p.stop:
			log.Debugf("stopping postgres keeper")
			cancel()
			p.pgm.Stop(true)
			p.end <- nil
			return
		case <-timerCh:
			go func() {
				p.postgresKeeperSM(ctx)
				endCh <- struct{}{}
			}()
		case <-endCh:
			timerCh = time.NewTimer(p.clusterConfig.SleepInterval).C
		case err := <-endApiCh:
			if err != nil {
				log.Fatal("ListenAndServe: ", err)
			}
			endCh <- struct{}{}
			return
		}
	}
}

func (p *PostgresKeeper) fullResync(master *cluster.MemberState, initialized, started bool) error {
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
		return fmt.Errorf("err: %v", err)
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
			log.Debugf("master timeline %d forked at xlog pos %d before our current state (timeline %d at xlog pos %d)", mPGState.TimelineID, tlh.SwitchPoint, pgState.TimelineID, pgState.XLogPos)
			return true
		}
	}
	return false
}

func (p *PostgresKeeper) postgresKeeperSM(pctx context.Context) {
	e := p.e
	pgm := p.pgm

	// Update cluster config
	clusterConfig, _, err := e.GetClusterConfig()
	if err != nil {
		log.Errorf("cannot get cluster config: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("clusterConfig: %+v", clusterConfig))
	// This shouldn't need a lock
	p.clusterConfig = clusterConfig

	cv, _, err := e.GetClusterView()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("clusterView: %v", cv))

	membersState, _, err := e.GetMembersState()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	if membersState == nil {
		membersState = cluster.MembersState{}
	}
	log.Debugf(spew.Sprintf("membersState: %v", membersState))

	member := membersState[p.id]
	log.Debugf(spew.Sprintf("member: %v", member))

	initialized, err := pgm.IsInitialized()
	if err != nil {
		log.Errorf("failed to detect if instance is initialized: %v", err)
		return
	}

	if cv == nil {
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
				log.Errorf("err: %v", err)
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
	log.Debugf("masterID: %s", masterID)

	master := membersState[masterID]
	log.Debugf(spew.Sprintf("master: %v", master))

	followersIDs := cv.GetFollowersIDs(p.id)

	memberRole, ok := cv.MembersRole[p.id]
	if !ok {
		log.Infof("our member state is not available")
		return
	}
	if memberRole.Follow == "" {
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
					log.Infof("dropping replication slot for member %q not marked as follower", slotName)
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
		log.Infof("our cluster requested state is standby following %q", memberRole.Follow)
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
			log.Debugf(spew.Sprintf("curConnParams: %s", curConnParams))

			replConnString := p.getReplConnString(master)
			newConnParams, err := pg.URLToConnParams(replConnString)
			log.Debugf(spew.Sprintf("newConnParams: %s", newConnParams))
			if err != nil {
				log.Errorf("err: %v", err)
				return
			}

			// Check that we can sync with master

			// Check timeline history
			ctx, cancel := context.WithTimeout(context.Background(), p.clusterConfig.RequestTimeout)
			pgState, err := pg.GetPGState(ctx, p.getOurReplConnString())
			cancel()
			if err != nil {
				log.Errorf("cannot get our pgstate: %v", err)
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
				log.Infof("following %s with connection url %s", memberRole.Follow, replConnString)
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
	log.Debugf("Got signal: %s", s)
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
		}
		if err := saveIDToFile(cfg, id); err != nil {
			log.Fatalf("error: %v", err)
		}
		log.Infof("generated id: %s", id)
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
