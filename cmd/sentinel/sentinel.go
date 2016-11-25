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
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/flagutil"
	"github.com/sorintlab/stolon/pkg/store"
	"github.com/sorintlab/stolon/pkg/timer"

	"github.com/davecgh/go-spew/spew"
	"github.com/docker/leadership"
	"github.com/mitchellh/copystructure"
	"github.com/spf13/cobra"
	"github.com/uber-go/zap"
	"golang.org/x/net/context"
)

var cmdSentinel = &cobra.Command{
	Use: "stolon-sentinel",
	Run: sentinel,
}

type config struct {
	storeBackend           string
	storeEndpoints         string
	storeCertFile          string
	storeKeyFile           string
	storeCAFile            string
	storeSkipTlsVerify     bool
	clusterName            string
	initialClusterSpecFile string
	debug                  bool
}

var cfg config

func init() {
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeBackend, "store-backend", "", "store backend type (etcd or consul)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (use https scheme for tls communication) (defaults: http://127.0.0.1:2379 for etcd, http://127.0.0.1:8500 for consul)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeCertFile, "store-cert-file", "", "certificate file for client identification to the store")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeKeyFile, "store-key", "", "private key file for client identification to the store")
	cmdSentinel.PersistentFlags().BoolVar(&cfg.storeSkipTlsVerify, "store-skip-tls-verify", false, "skip store certificate verification (insecure!!!)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeCAFile, "store-ca-file", "", "verify certificates of HTTPS-enabled store servers using this CA bundle")
	cmdSentinel.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdSentinel.PersistentFlags().StringVar(&cfg.initialClusterSpecFile, "initial-cluster-spec", "", "a file providing the initial cluster specification, used only at cluster initialization, ignored if cluster is already initialized")
	cmdSentinel.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
}

var log = zap.New(zap.NewTextEncoder(), zap.AddCaller())

func (s *Sentinel) electionLoop() {
	for {
		log.Info("Trying to acquire sentinels leadership")
		electedCh, errCh := s.candidate.RunForElection()
		for {
			select {
			case elected := <-electedCh:
				s.leaderMutex.Lock()
				if elected {
					log.Info("sentinel leadership acquired")
					s.leader = true
					s.leadershipCount++
				} else {
					if s.leader {
						log.Info("sentinel leadership lost")
					}
					s.leader = false
				}
				s.leaderMutex.Unlock()

			case err := <-errCh:
				if err != nil {
					log.Error("election loop error", zap.Error(err))
				}
				goto end
			case <-s.stop:
				log.Debug("stopping election Loop")
				return
			}
		}
	end:
		time.Sleep(10 * time.Second)
	}
}

func (s *Sentinel) setSentinelInfo(ttl time.Duration) error {
	sentinelInfo := &cluster.SentinelInfo{
		UID: s.id,
	}
	log.Debug("sentinelInfod dump", zap.String("sentinelInfo", spew.Sdump(sentinelInfo)))

	if err := s.e.SetSentinelInfo(sentinelInfo, ttl); err != nil {
		return err
	}
	return nil
}

func (s *Sentinel) findBestStandby(cd *cluster.ClusterData, masterDB *cluster.DB) (*cluster.DB, error) {
	var bestDB *cluster.DB
	for _, db := range cd.DBs {
		if db.UID == masterDB.UID {
			log.Debug("ignoring db since it's the current master", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
			continue
		}
		if db.Status.SystemID != masterDB.Status.SystemID {
			log.Debug("ignoring db since the postgres systemdID is different that the master one", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID), zap.String("dbSystemdID", db.Status.SystemID), zap.String("masterSystemID", masterDB.Status.SystemID))
			continue

		}
		if !db.Status.Healthy {
			log.Debug("ignoring db since it's not healthy", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
			continue
		}
		if db.Status.CurrentGeneration != db.Generation {
			log.Debug("ignoring keeper since its generation is different that the current one", zap.String("db", db.UID), zap.Int64("currentGeneration", db.Status.CurrentGeneration), zap.Int64("generation", db.Generation))
			continue
		}
		if db.Status.TimelineID != masterDB.Status.TimelineID {
			log.Debug("ignoring keeper since its pg timeline is different than master timeline", zap.String("db", db.UID), zap.Uint64("dbTimeline", db.Status.TimelineID), zap.Uint64("masterTimeline", masterDB.Status.TimelineID))
			continue
		}
		if bestDB == nil {
			bestDB = db
			continue
		}
		if db.Status.XLogPos > bestDB.Status.XLogPos {
			bestDB = db
		}
	}
	if bestDB == nil {
		return nil, fmt.Errorf("no standbys available")
	}
	return bestDB, nil
}

func (s *Sentinel) getKeepersInfo(ctx context.Context) (cluster.KeepersInfo, error) {
	return s.e.GetKeepersInfo()
}

func (s *Sentinel) SetKeeperError(id string) {
	if _, ok := s.keeperErrorTimers[id]; !ok {
		s.keeperErrorTimers[id] = timer.Now()
	}
}

func (s *Sentinel) CleanKeeperError(id string) {
	if _, ok := s.keeperErrorTimers[id]; ok {
		delete(s.keeperErrorTimers, id)
	}
}

func (s *Sentinel) SetDBError(id string) {
	if _, ok := s.dbErrorTimers[id]; !ok {
		s.dbErrorTimers[id] = timer.Now()
	}
}

func (s *Sentinel) CleanDBError(id string) {
	if _, ok := s.dbErrorTimers[id]; ok {
		delete(s.dbErrorTimers, id)
	}
}

func (s *Sentinel) updateKeepersStatus(cd *cluster.ClusterData, keepersInfo cluster.KeepersInfo, firstRun bool) (*cluster.ClusterData, KeeperInfoHistories) {
	// Create a copy of cd
	cd = cd.DeepCopy()

	kihs := s.keeperInfoHistories.DeepCopy()

	// Remove keepers with wrong cluster UID
	tmpKeepersInfo := keepersInfo.DeepCopy()
	for _, ki := range keepersInfo {
		if ki.ClusterUID != cd.Cluster.UID {
			delete(tmpKeepersInfo, ki.UID)
		}
	}
	keepersInfo = tmpKeepersInfo

	// On first run just insert keepers info in the history with Seen set
	// to false and don't do any change to the keepers' state
	if firstRun {
		for keeperUID, ki := range keepersInfo {
			kihs[keeperUID] = &KeeperInfoHistory{KeeperInfo: ki, Seen: false}
		}
		return cd, kihs
	}

	tmpKeepersInfo = keepersInfo.DeepCopy()
	// keep only updated keepers info
	for keeperUID, ki := range keepersInfo {
		if kih, ok := kihs[keeperUID]; ok {
			log.Debug("kih", zap.Object("kih", kih))
			if kih.KeeperInfo.InfoUID == ki.InfoUID {
				if !kih.Seen {
					//Remove since it was already there and wasn't updated
					delete(tmpKeepersInfo, ki.UID)
				} else if kih.Seen && timer.Since(kih.Timer) > s.sleepInterval {
					//Remove since it wasn't updated
					delete(tmpKeepersInfo, ki.UID)
				}
			}
			if kih.KeeperInfo.InfoUID != ki.InfoUID {
				kihs[keeperUID] = &KeeperInfoHistory{KeeperInfo: ki, Seen: true, Timer: timer.Now()}
			}
		} else {
			kihs[keeperUID] = &KeeperInfoHistory{KeeperInfo: ki, Seen: true, Timer: timer.Now()}
		}
	}
	keepersInfo = tmpKeepersInfo

	// Create new keepers from keepersInfo
	for keeperUID, ki := range keepersInfo {
		if _, ok := cd.Keepers[keeperUID]; !ok {
			k := cluster.NewKeeperFromKeeperInfo(ki)
			cd.Keepers[k.UID] = k
		}
	}

	// Mark keepers without a keeperInfo (cleaned up above from not updated
	// ones) as in error
	for keeperUID, k := range cd.Keepers {
		if ki, ok := keepersInfo[keeperUID]; !ok {
			s.SetKeeperError(keeperUID)
		} else {
			s.CleanKeeperError(keeperUID)
			// Update keeper status infos
			k.Status.BootUUID = ki.BootUUID
		}
	}

	// Update keepers' healthy states
	for _, k := range cd.Keepers {
		k.Status.Healthy = s.isKeeperHealthy(cd, k)
	}

	// Update dbs' states
	for _, db := range cd.DBs {
		// Mark not found DBs in DBstates in error
		k, ok := keepersInfo[db.Spec.KeeperUID]
		if !ok {
			log.Error("no keeper info available", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
			s.SetDBError(db.UID)
			continue
		}
		dbs := k.PostgresState
		if dbs == nil {
			log.Error("no db state available", zap.String("db", db.UID))
			s.SetDBError(db.UID)
			continue
		}
		if dbs.UID != db.UID {
			log.Warn("received db state for unexpected db uid", zap.String("receivedDB", dbs.UID), zap.String("db", db.UID))
			s.SetDBError(db.UID)
			continue
		}
		log.Debug("received db state", zap.String("db", db.UID))
		db.Status.ListenAddress = dbs.ListenAddress
		db.Status.Port = dbs.Port
		db.Status.CurrentGeneration = dbs.Generation
		if dbs.Healthy {
			s.CleanDBError(db.UID)
			db.Status.SystemID = dbs.SystemID
			db.Status.TimelineID = dbs.TimelineID
			db.Status.XLogPos = dbs.XLogPos
			db.Status.TimelinesHistory = dbs.TimelinesHistory
			db.Status.PGParameters = cluster.PGParameters(dbs.PGParameters)
		} else {
			s.SetDBError(db.UID)
		}
	}

	// Update dbs' healthy state
	for _, db := range cd.DBs {
		db.Status.Healthy = s.isDBHealthy(cd, db)
	}

	return cd, kihs
}

func (s *Sentinel) findInitialKeeper(cd *cluster.ClusterData) (*cluster.Keeper, error) {
	if len(cd.Keepers) < 1 {
		return nil, fmt.Errorf("no keepers registered")
	}
	r := s.RandFn(len(cd.Keepers))
	keys := []string{}
	for k, _ := range cd.Keepers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return cd.Keepers[keys[r]], nil
}

func (s *Sentinel) setDBSpecFromClusterSpec(cd *cluster.ClusterData) {
	// Update dbSpec values with the related clusterSpec ones
	for _, db := range cd.DBs {
		db.Spec.RequestTimeout = cd.Cluster.Spec.RequestTimeout
		db.Spec.MaxStandbys = cd.Cluster.Spec.MaxStandbys
		db.Spec.SynchronousReplication = cd.Cluster.Spec.SynchronousReplication
		db.Spec.UsePgrewind = cd.Cluster.Spec.UsePgrewind
		db.Spec.PGParameters = cd.Cluster.Spec.PGParameters
	}
}

func (s *Sentinel) freeKeepers(cd *cluster.ClusterData) []*cluster.Keeper {
	freeKeepers := []*cluster.Keeper{}
K:
	for _, keeper := range cd.Keepers {
		if !keeper.Status.Healthy {
			continue
		}
		for _, db := range cd.DBs {
			if db.Spec.KeeperUID == keeper.UID {
				continue K
			}
		}
		freeKeepers = append(freeKeepers, keeper)
	}
	return freeKeepers
}

func (s *Sentinel) standbys(cd *cluster.ClusterData) map[string]*cluster.DB {
	// A standby is a db that:
	// * Isn't the master
	// * Is an "internal standby". Not a db marked to replicate from an external db
	standbys := map[string]*cluster.DB{}
	for _, db := range cd.DBs {
		if db.UID == cd.Cluster.Status.Master {
			continue
		}
		if db.Spec.Role != common.RoleStandby {
			continue
		}
		// Skip db with follow type != internal (for example a master in a standby stolon cluster)
		if db.Spec.FollowConfig.Type != cluster.FollowTypeInternal {
			continue
		}
		standbys[db.UID] = db
	}
	return standbys
}

func (s *Sentinel) standbysByStatus(cd *cluster.ClusterData) (map[string]*cluster.DB, map[string]*cluster.DB, map[string]*cluster.DB) {
	goodStandbys := map[string]*cluster.DB{}
	failedStandbys := map[string]*cluster.DB{}
	convergingStandbys := map[string]*cluster.DB{}
	for _, db := range s.standbys(cd) {
		// if keeper failed then mark as failed
		keeper := cd.Keepers[db.Spec.KeeperUID]
		if !keeper.Status.Healthy {
			failedStandbys[db.UID] = db
			continue
		}
		// TODO(sgotti) find a way to determine if the db is syncing or
		// is doing a base convergence (i.e. changed parameters)
		convergenceState := s.dbConvergenceState(db, cd.Cluster.Spec.SyncTimeout.Duration)
		// if converging then it's not failed (it can also be not healthy since it could be resyncing
		// if convergence failed then mark as failed
		switch convergenceState {
		case ConvergenceFailed:
			failedStandbys[db.UID] = db
			continue
		case Converging:
			convergingStandbys[db.UID] = db
			continue
		}
		// if converged but not healthy mark as failed
		if !db.Status.Healthy {
			failedStandbys[db.UID] = db
			continue
		}

		// db is good
		goodStandbys[db.UID] = db
	}
	return goodStandbys, failedStandbys, convergingStandbys
}

func (s *Sentinel) updateCluster(cd *cluster.ClusterData) (*cluster.ClusterData, error) {
	newcd := cd.DeepCopy()
	switch cd.Cluster.Status.Phase {
	case cluster.ClusterPhaseInitializing:
		switch cd.Cluster.Spec.InitMode {
		case cluster.ClusterInitModeNew:
			// Is there already a keeper choosed to be the new master?
			if cd.Cluster.Status.Master == "" {
				log.Info("trying to find initial master")
				k, err := s.findInitialKeeper(cd)
				if err != nil {
					return nil, fmt.Errorf("cannot choose initial master: %v", err)
				}
				log.Info("initializing cluster", zap.String("keeper", k.UID))
				db := &cluster.DB{
					UID:        s.UIDFn(),
					Generation: cluster.InitialGeneration,
					ChangeTime: time.Now(),
					Spec: &cluster.DBSpec{
						KeeperUID:     k.UID,
						InitMode:      cluster.DBInitModeNew,
						Role:          common.RoleMaster,
						Followers:     []string{},
						IncludeConfig: *cd.Cluster.Spec.MergePgParameters,
					},
				}
				newcd.DBs[db.UID] = db
				newcd.Cluster.Status.Master = db.UID
				log.Debug("newcd dump", zap.String("newcd", spew.Sdump(newcd)))
			} else {
				db, ok := cd.DBs[cd.Cluster.Status.Master]
				if !ok {
					panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
				}
				// Check that the choosed db for being the master has correctly initialized
				switch s.dbConvergenceState(db, cd.Cluster.Spec.InitTimeout.Duration) {
				case Converged:
					if db.Status.Healthy {
						log.Info("db initialized", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
						// Set db initMode to none, not needed but just a security measure
						db.Spec.InitMode = cluster.DBInitModeNone
						// Don't include previous config anymore
						db.Spec.IncludeConfig = false
						// Replace reported pg parameters in cluster spec
						if *cd.Cluster.Spec.MergePgParameters {
							newcd.Cluster.Spec.PGParameters = db.Status.PGParameters
						}
						// Cluster initialized, switch to Normal state
						newcd.Cluster.Status.Phase = cluster.ClusterPhaseNormal
					}
				case Converging:
					log.Info("waiting for db", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
				case ConvergenceFailed:
					log.Info("db failed to initialize", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
					// Empty DBs
					newcd.DBs = cluster.DBs{}
					// Unset master so another keeper can be choosen
					newcd.Cluster.Status.Master = ""
				}
			}
		case cluster.ClusterInitModeExisting:
			if cd.Cluster.Status.Master == "" {
				wantedKeeper := cd.Cluster.Spec.ExistingConfig.KeeperUID
				log.Info("trying to use keeper as initial master", zap.String("keeper", wantedKeeper))

				k, ok := cd.Keepers[wantedKeeper]
				if !ok {
					return nil, fmt.Errorf("keeper %q state not available", wantedKeeper)
				}

				log.Info("initializing cluster using selected keeper as master db owner", zap.String("keeper", k.UID))

				db := &cluster.DB{
					UID:        s.UIDFn(),
					Generation: cluster.InitialGeneration,
					ChangeTime: time.Now(),
					Spec: &cluster.DBSpec{
						KeeperUID:     k.UID,
						InitMode:      cluster.DBInitModeExisting,
						Role:          common.RoleMaster,
						Followers:     []string{},
						IncludeConfig: *cd.Cluster.Spec.MergePgParameters,
					},
				}
				newcd.DBs[db.UID] = db
				newcd.Cluster.Status.Master = db.UID
				log.Debug("newcd dump", zap.String("newcd", spew.Sdump(newcd)))
			} else {
				db, ok := newcd.DBs[cd.Cluster.Status.Master]
				if !ok {
					panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
				}
				// Check that the choosed db for being the master has correctly initialized
				if db.Status.Healthy && s.dbConvergenceState(db, cd.Cluster.Spec.ConvergenceTimeout.Duration) == Converged {
					log.Info("db initialized", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
					// Don't include previous config anymore
					db.Spec.IncludeConfig = false
					// Replace reported pg parameters in cluster spec
					if *cd.Cluster.Spec.MergePgParameters {
						newcd.Cluster.Spec.PGParameters = db.Status.PGParameters
					}
					// Cluster initialized, switch to Normal state
					newcd.Cluster.Status.Phase = cluster.ClusterPhaseNormal
				}
			}
		case cluster.ClusterInitModePITR:
			// Is there already a keeper choosed to be the new master?
			if cd.Cluster.Status.Master == "" {
				log.Info("trying to find initial master")
				k, err := s.findInitialKeeper(cd)
				if err != nil {
					return nil, fmt.Errorf("cannot choose initial master: %v", err)
				}
				log.Info("initializing cluster using selected keeper as master db owner", zap.String("keeper", k.UID))
				db := &cluster.DB{
					UID:        s.UIDFn(),
					Generation: cluster.InitialGeneration,
					ChangeTime: time.Now(),
					Spec: &cluster.DBSpec{
						KeeperUID:     k.UID,
						InitMode:      cluster.DBInitModePITR,
						PITRConfig:    cd.Cluster.Spec.PITRConfig,
						Role:          common.RoleMaster,
						Followers:     []string{},
						IncludeConfig: *cd.Cluster.Spec.MergePgParameters,
					},
				}
				newcd.DBs[db.UID] = db
				newcd.Cluster.Status.Master = db.UID
				log.Debug("newcd dump", zap.String("newcd", spew.Sdump(newcd)))
			} else {
				db, ok := cd.DBs[cd.Cluster.Status.Master]
				if !ok {
					panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
				}
				// Check that the choosed db for being the master has correctly initialized
				// TODO(sgotti) set a timeout (the max time for a restore operation)
				switch s.dbConvergenceState(db, 0) {
				case Converged:
					if db.Status.Healthy {
						log.Info("db initialized", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
						// Set db initMode to none, not needed but just a security measure
						db.Spec.InitMode = cluster.DBInitModeNone
						// Don't include previous config anymore
						db.Spec.IncludeConfig = false
						// Replace reported pg parameters in cluster spec
						if *cd.Cluster.Spec.MergePgParameters {
							newcd.Cluster.Spec.PGParameters = db.Status.PGParameters
						}
						// Cluster initialized, switch to Normal state
						newcd.Cluster.Status.Phase = cluster.ClusterPhaseNormal
					}
				case Converging:
					log.Info("waiting for db to converge", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
				case ConvergenceFailed:
					log.Info("db failed to initialize", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
					// Empty DBs
					newcd.DBs = cluster.DBs{}
					// Unset master so another keeper can be choosen
					newcd.Cluster.Status.Master = ""
				}
			}
		default:
			return nil, fmt.Errorf("unknown init mode %q", cd.Cluster.Spec.InitMode)
		}
	case cluster.ClusterPhaseNormal:
		// TODO(sgotti) When keeper removal is implemented, remove DBs for unexistent keepers

		// Calculate current master status
		curMasterDBUID := cd.Cluster.Status.Master
		wantedMasterDBUID := curMasterDBUID

		masterOK := true
		curMasterDB := cd.DBs[curMasterDBUID]
		if curMasterDB == nil {
			return nil, fmt.Errorf("db for keeper %q not available. This shouldn't happen!", curMasterDBUID)
		}
		log.Debug("db dump", zap.String("db", spew.Sdump(curMasterDB)))

		if !curMasterDB.Status.Healthy {
			log.Info("master db is failed", zap.String("db", curMasterDB.UID), zap.String("keeper", curMasterDB.Spec.KeeperUID))
			masterOK = false
		}

		// Check that the wanted master is in master state (i.e. check that promotion from standby to master happened)
		if s.dbConvergenceState(curMasterDB, newcd.Cluster.Spec.ConvergenceTimeout.Duration) == ConvergenceFailed {
			log.Info("db not converged", zap.String("db", curMasterDB.UID), zap.String("keeper", curMasterDB.Spec.KeeperUID))
			masterOK = false
		}

		if !masterOK {
			log.Info("trying to find a standby to replace failed master")
			bestStandbyDB, err := s.findBestStandby(cd, curMasterDB)
			if err != nil {
				log.Error("error trying to find the best standby", zap.Error(err))
			} else {
				log.Info("electing db as the new master", zap.String("db", bestStandbyDB.UID), zap.String("keeper", bestStandbyDB.Spec.KeeperUID))
				wantedMasterDBUID = bestStandbyDB.UID
			}
		}

		// New master elected
		if curMasterDBUID != wantedMasterDBUID {
			// maintain the current role, remove followers
			oldMasterdb := newcd.DBs[curMasterDBUID]
			oldMasterdb.Spec.Followers = []string{}

			newcd.Cluster.Status.Master = wantedMasterDBUID
			newMasterDB := newcd.DBs[wantedMasterDBUID]
			newMasterDB.Spec.Role = common.RoleMaster
			newMasterDB.Spec.FollowConfig = nil

			// Tell proxy that there's currently no active master
			newcd.Proxy.Spec.MasterDBUID = ""
			newcd.Proxy.ChangeTime = time.Now()
		}

		// TODO(sgotti) Wait for the proxies being converged (closed connections to old master)?

		// Setup standbys, do this only when there's no master change
		if curMasterDBUID == wantedMasterDBUID {
			masterDB := newcd.DBs[curMasterDBUID]
			// Set standbys to follow master only if it's healthy and converged
			if masterDB.Status.Healthy && s.dbConvergenceState(masterDB, newcd.Cluster.Spec.ConvergenceTimeout.Duration) == Converged {
				// Tell proxy that there's a new active master
				newcd.Proxy.Spec.MasterDBUID = wantedMasterDBUID
				newcd.Proxy.ChangeTime = time.Now()

				standbys := s.standbys(newcd)
				standbysCount := len(standbys)

				goodStandbys, failedStandbys, convergingStandbys := s.standbysByStatus(newcd)
				goodStandbysCount := len(goodStandbys)
				failedStandbysCount := len(failedStandbys)
				convergingStandbysCount := len(convergingStandbys)
				log.Debug("standbys states", zap.Int("n", goodStandbysCount), zap.Int("failed", failedStandbysCount), zap.Int("converging", convergingStandbysCount))

				// NotFailed != Good since there can be some dbs that are converging
				// it's the total number of standbys - the failed standbys
				// or the sum of good + converging standbys
				notFailedStandbysCount := standbysCount - failedStandbysCount

				// Add new dbs to substitute failed dbs. we
				// don't remove failed db until the number of
				// good db is >= MaxStandbysPerSender since they can come back

				// define, if there're available keepers, new dbs
				// nc can be negative if MaxStandbysPerSender has been lowered
				nc := int(newcd.Cluster.Spec.MaxStandbysPerSender - uint16(notFailedStandbysCount))
				// Add missing DBs until MaxStandbysPerSender
				freeKeepers := s.freeKeepers(newcd)
				nf := len(freeKeepers)
				for i := 0; i < nc && i < nf; i++ {
					freeKeeper := freeKeepers[i]
					db := &cluster.DB{
						UID:        s.UIDFn(),
						Generation: cluster.InitialGeneration,
						ChangeTime: time.Now(),
						Spec: &cluster.DBSpec{
							KeeperUID: freeKeeper.UID,
							InitMode:  cluster.DBInitModeNone,
							Role:      common.RoleUndefined,
							Followers: []string{},
						},
					}
					newcd.DBs[db.UID] = db
					log.Info("added new standby db", zap.String("db", db.UID), zap.String("keeper", db.Spec.KeeperUID))
				}

				// Remove dbs if we have a good number >= MaxStandbysPerSender
				if uint16(goodStandbysCount) >= newcd.Cluster.Spec.MaxStandbysPerSender {
					toRemove := []*cluster.DB{}
					log.Info("removing non good standbys", zap.Int("n", standbysCount-goodStandbysCount))
					// Remove all non good standbys
					for _, db := range standbys {
						if _, ok := goodStandbys[db.UID]; !ok {
							toRemove = append(toRemove, db)
						}
					}
					// Remove good standbys in excess
					nr := int(uint16(goodStandbysCount) - newcd.Cluster.Spec.MaxStandbysPerSender)
					log.Info("removing good standbys in excess", zap.Int("n", nr))
					i := 0
					for _, db := range goodStandbys {
						if i >= nr {
							break
						}
						toRemove = append(toRemove, db)
						i++
					}
					for _, db := range toRemove {
						delete(newcd.DBs, db.UID)
					}

				}

				// Reconfigure all db except the master as followers of the current master
				for id, db := range newcd.DBs {
					if id == wantedMasterDBUID {
						continue
					}

					db.Spec.Role = common.RoleStandby
					// Remove followers
					db.Spec.Followers = []string{}
					db.Spec.FollowConfig = &cluster.FollowConfig{Type: cluster.FollowTypeInternal, DBUID: wantedMasterDBUID}
				}

				// Define followers for master DB
				masterDB.Spec.Followers = []string{}
				for _, db := range newcd.DBs {
					if masterDB.UID == db.UID {
						continue
					}
					fc := db.Spec.FollowConfig
					if fc != nil {
						if fc.Type == cluster.FollowTypeInternal && fc.DBUID == wantedMasterDBUID {
							masterDB.Spec.Followers = append(masterDB.Spec.Followers, db.UID)
							// Sort followers so the slice won't be considered as changed due to different order of the same entries.
							sort.Strings(masterDB.Spec.Followers)
						}
					}
				}
			}
		}

		// Update generation on DBs if they have changed
		for dbUID, db := range newcd.DBs {
			prevDB, ok := cd.DBs[dbUID]
			if !ok {
				continue
			}
			if !reflect.DeepEqual(db.Spec, prevDB.Spec) {
				log.Debug("db spec changed, updating generation", zap.String("prevDB", spew.Sdump(prevDB.Spec)), zap.String("db", spew.Sdump(db.Spec)))
				db.Generation++
				db.ChangeTime = time.Now()
			}
		}

	default:
		return nil, fmt.Errorf("unknown cluster phase %s", cd.Cluster.Status.Phase)
	}

	// Copy the clusterSpec parameters to the dbSpec
	s.setDBSpecFromClusterSpec(newcd)

	return newcd, nil
}

type ConvergenceState uint

const (
	Converging ConvergenceState = iota
	Converged
	ConvergenceFailed
)

func (s *Sentinel) isKeeperHealthy(cd *cluster.ClusterData, keeper *cluster.Keeper) bool {
	t, ok := s.keeperErrorTimers[keeper.UID]
	if !ok {
		return true
	}
	if timer.Since(t) > cd.Cluster.Spec.FailInterval.Duration {
		return false
	}
	return true
}

func (s *Sentinel) isDBHealthy(cd *cluster.ClusterData, db *cluster.DB) bool {
	t, ok := s.dbErrorTimers[db.UID]
	if !ok {
		return true
	}
	if timer.Since(t) > cd.Cluster.Spec.FailInterval.Duration {
		return false
	}
	return true
}

func (s *Sentinel) updateDBConvergenceInfos(cd *cluster.ClusterData) {
	for _, db := range cd.DBs {
		if db.Status.CurrentGeneration == db.Generation {
			delete(s.dbConvergenceInfos, db.UID)
			continue
		}
		nd := &DBConvergenceInfo{Generation: db.Generation, Timer: timer.Now()}
		d, ok := s.dbConvergenceInfos[db.UID]
		if !ok {
			s.dbConvergenceInfos[db.UID] = nd
		} else if d.Generation != db.Generation {
			s.dbConvergenceInfos[db.UID] = nd
		}
	}
}

func (s *Sentinel) dbConvergenceState(db *cluster.DB, timeout time.Duration) ConvergenceState {
	if db.Status.CurrentGeneration == db.Generation {
		return Converged
	}
	if timeout != 0 {
		d, ok := s.dbConvergenceInfos[db.UID]
		if !ok {
			panic(fmt.Errorf("no db convergence info for db %q, this shouldn't happen!", db.UID))
		}
		if timer.Since(d.Timer) > timeout {
			return ConvergenceFailed
		}
	}
	return Converging
}

type KeeperInfoHistory struct {
	KeeperInfo *cluster.KeeperInfo
	Seen       bool
	Timer      int64
}

type KeeperInfoHistories map[string]*KeeperInfoHistory

func (k KeeperInfoHistories) DeepCopy() KeeperInfoHistories {
	if k == nil {
		return nil
	}
	nk, err := copystructure.Copy(k)
	if err != nil {
		panic(err)
	}
	log.Debug("", zap.String("k", spew.Sdump(k)), zap.String("nk", spew.Sdump(nk)))
	if !reflect.DeepEqual(k, nk) {
		panic("not equal")
	}
	return nk.(KeeperInfoHistories)
}

type DBConvergenceInfo struct {
	Generation int64
	Timer      int64
}

type Sentinel struct {
	id  string
	cfg *config
	e   *store.StoreManager

	candidate *leadership.Candidate
	stop      chan bool
	end       chan bool

	lastLeadershipCount uint

	updateMutex sync.Mutex
	leader      bool
	// Used to determine if we lost and regained the leadership
	leadershipCount uint
	leaderMutex     sync.Mutex

	initialClusterSpec *cluster.ClusterSpec

	sleepInterval  time.Duration
	requestTimeout time.Duration

	// Make UIDFn settable to ease testing with reproducible UIDs
	UIDFn func() string
	// Make RandFn settable to ease testing with reproducible "random" numbers
	RandFn func(int) int

	keeperErrorTimers  map[string]int64
	dbErrorTimers      map[string]int64
	dbConvergenceInfos map[string]*DBConvergenceInfo

	keeperInfoHistories KeeperInfoHistories
}

func NewSentinel(id string, cfg *config, stop chan bool, end chan bool) (*Sentinel, error) {
	var initialClusterSpec *cluster.ClusterSpec
	if cfg.initialClusterSpecFile != "" {
		configData, err := ioutil.ReadFile(cfg.initialClusterSpecFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read provided initial cluster config file: %v", err)
		}
		if err := json.Unmarshal(configData, &initialClusterSpec); err != nil {
			return nil, fmt.Errorf("cannot parse provided initial cluster config: %v", err)
		}
		initialClusterSpec.SetDefaults()
		log.Debug("initialClusterSpec dump", zap.String("initialClusterSpec", spew.Sdump(initialClusterSpec)))
		if err := initialClusterSpec.Validate(); err != nil {
			return nil, fmt.Errorf("invalid initial cluster: %v", err)
		}
	}

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

	candidate := leadership.NewCandidate(kvstore, filepath.Join(storePath, common.SentinelLeaderKey), id, store.MinTTL)

	return &Sentinel{
		id:                 id,
		cfg:                cfg,
		e:                  e,
		candidate:          candidate,
		leader:             false,
		initialClusterSpec: initialClusterSpec,
		stop:               stop,
		end:                end,
		UIDFn:              common.UID,
		// This is just to choose a pseudo random keeper so
		// use math.rand (no need for crypto.rand) without an
		// initial seed.
		RandFn: rand.Intn,

		sleepInterval:  cluster.DefaultSleepInterval,
		requestTimeout: cluster.DefaultRequestTimeout,
	}, nil
}

func (s *Sentinel) Start() {
	endCh := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	timerCh := time.NewTimer(0).C

	go s.electionLoop()

	for true {
		select {
		case <-s.stop:
			log.Debug("stopping stolon sentinel")
			cancel()
			s.candidate.Stop()
			s.end <- true
			return
		case <-timerCh:
			go func() {
				s.clusterSentinelCheck(ctx)
				endCh <- struct{}{}
			}()
		case <-endCh:
			timerCh = time.NewTimer(s.sleepInterval).C
		}
	}
}

func (s *Sentinel) leaderInfo() (bool, uint) {
	s.leaderMutex.Lock()
	defer s.leaderMutex.Unlock()
	return s.leader, s.leadershipCount
}

func (s *Sentinel) clusterSentinelCheck(pctx context.Context) {
	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()
	e := s.e

	cd, prevCDPair, err := e.GetClusterData()
	if err != nil {
		log.Error("error retrieving cluster data", zap.Error(err))
		return
	}
	if cd != nil {
		if cd.FormatVersion != cluster.CurrentCDFormatVersion {
			log.Error("unsupported clusterdata format version", zap.Uint64("version", cd.FormatVersion))
			return
		}
		if cd.Cluster != nil {
			s.sleepInterval = cd.Cluster.Spec.SleepInterval.Duration
			s.requestTimeout = cd.Cluster.Spec.RequestTimeout.Duration
		}

	}
	log.Debug("cd dump", zap.String("cd", spew.Sdump(cd)))

	if cd == nil {
		// Cluster first initialization
		if s.initialClusterSpec == nil {
			log.Info("no cluster data available, waiting for it to appear")
			return
		}
		c := cluster.NewCluster(s.UIDFn(), s.initialClusterSpec)
		log.Info("writing initial cluster data")
		newcd := cluster.NewClusterData(c)
		log.Debug("newcd dump", zap.String("newcd", spew.Sdump(newcd)))
		if _, err = e.AtomicPutClusterData(newcd, nil); err != nil {
			log.Error("error saving cluster data", zap.Error(err))
		}
		return
	}

	if err = s.setSentinelInfo(2 * s.sleepInterval); err != nil {
		log.Error("cannot update sentinel info", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(pctx, s.requestTimeout)
	keepersInfo, err := s.getKeepersInfo(ctx)
	cancel()
	if err != nil {
		log.Error("err", zap.Error(err))
		return
	}
	log.Debug("keepersInfo dump", zap.String("keepersInfo", spew.Sdump(keepersInfo)))

	isLeader, leadershipCount := s.leaderInfo()
	if !isLeader {
		return
	}

	// detect if this is the first check after (re)gaining leadership
	firstRun := false
	if s.lastLeadershipCount != leadershipCount {
		firstRun = true
		s.lastLeadershipCount = leadershipCount
	}

	// if this is the first check after (re)gaining leadership reset all
	// the internal timers
	if firstRun {
		s.keeperErrorTimers = make(map[string]int64)
		s.dbErrorTimers = make(map[string]int64)
		s.keeperInfoHistories = make(KeeperInfoHistories)
		s.dbConvergenceInfos = make(map[string]*DBConvergenceInfo)

		// Update db convergence timers since its the first run
		s.updateDBConvergenceInfos(cd)
	}

	newcd, newKeeperInfoHistories := s.updateKeepersStatus(cd, keepersInfo, firstRun)

	newcd, err = s.updateCluster(newcd)
	if err != nil {
		log.Error("failed to update cluster data", zap.Error(err))
		return
	}
	log.Debug("newcd dump after updateCluster", zap.String("newcd", spew.Sdump(newcd)))

	if newcd != nil {
		if _, err := e.AtomicPutClusterData(newcd, prevCDPair); err != nil {
			log.Error("error saving clusterdata", zap.Error(err))
		}
	}

	// Save the new keeperInfoHistories only on successfull cluster data
	// update or in the next run we'll think that the saved keeperInfo was
	// already applied.
	s.keeperInfoHistories = newKeeperInfoHistories

	// Update db convergence timers using the new cluster data
	s.updateDBConvergenceInfos(newcd)
}

func sigHandler(sigs chan os.Signal, stop chan bool) {
	s := <-sigs
	log.Debug("got signal", zap.Stringer("signal", s))
	close(stop)
}

func main() {
	flagutil.SetFlagsFromEnv(cmdSentinel.PersistentFlags(), "STSENTINEL")

	cmdSentinel.Execute()
}

func sentinel(cmd *cobra.Command, args []string) {
	if cfg.debug {
		log.SetLevel(zap.DebugLevel)
	}
	if cfg.clusterName == "" {
		fmt.Println("cluster name required")
		os.Exit(1)
	}
	if cfg.storeBackend == "" {
		fmt.Println("store backend type required")
		os.Exit(1)

	}

	id := common.UID()
	log.Info("sentinel id", zap.String("id", id))

	stop := make(chan bool, 0)
	end := make(chan bool, 0)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill)
	go sigHandler(sigs, stop)

	s, err := NewSentinel(id, &cfg, stop, end)
	if err != nil {
		fmt.Printf("cannot create sentinel: %v\n", err)
		os.Exit(1)
	}
	go s.Start()

	<-end
}
