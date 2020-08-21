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
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sorintlab/stolon/cmd"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/common"
	"github.com/sorintlab/stolon/internal/flagutil"
	slog "github.com/sorintlab/stolon/internal/log"
	pg "github.com/sorintlab/stolon/internal/postgresql"
	"github.com/sorintlab/stolon/internal/store"
	"github.com/sorintlab/stolon/internal/timer"
	"github.com/sorintlab/stolon/internal/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/mitchellh/copystructure"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var log = slog.S()

const (
	fakeStandbyName = "stolonfakestandby"
)

var CmdSentinel = &cobra.Command{
	Use:     "stolon-sentinel",
	Run:     sentinel,
	Version: cmd.Version,
}

type config struct {
	cmd.CommonConfig
	initialClusterSpecFile string
	debug                  bool
}

var cfg config

func init() {
	cmd.AddCommonFlags(CmdSentinel, &cfg.CommonConfig)

	CmdSentinel.PersistentFlags().StringVar(&cfg.initialClusterSpecFile, "initial-cluster-spec", "", "a file providing the initial cluster specification, used only at cluster initialization, ignored if cluster is already initialized")
	CmdSentinel.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging (deprecated, use log-level instead)")

	if err := CmdSentinel.PersistentFlags().MarkDeprecated("debug", "use --log-level=debug instead"); err != nil {
		log.Fatal(err)
	}
}

func (s *Sentinel) electionLoop(ctx context.Context) {
	for {
		log.Infow("Trying to acquire sentinels leadership")
		electedCh, errCh := s.election.RunForElection()
		for {
			select {
			case elected := <-electedCh:
				s.leaderMutex.Lock()
				if elected {
					log.Infow("sentinel leadership acquired")
					s.leader = true
					s.leadershipCount++
				} else {
					if s.leader {
						log.Infow("sentinel leadership lost")
					}
					s.leader = false
				}
				s.leaderMutex.Unlock()

			case err := <-errCh:
				if err != nil {
					log.Errorw("election loop error", zap.Error(err))

					// It's important to Stop() any on-going elections, as most stores will block
					// until all previous elections have completed. If we continue without stopping,
					// we run the risk of preventing any subsequent elections from successfully
					// electing a leader.
					s.election.Stop()
				}
				goto end
			case <-ctx.Done():
				log.Debugw("stopping election loop")
				s.election.Stop()
				return
			}
		}
	end:
		time.Sleep(10 * time.Second)
	}
}

// syncRepl return whether to use synchronous replication based on the current
// cluster spec.
func (s *Sentinel) syncRepl(spec *cluster.ClusterSpec) bool {
	// a cluster standby role means our "master" will act as a cascading standby to
	// the other keepers, in this case we can't use synchronous replication
	return *spec.SynchronousReplication && *spec.Role == cluster.ClusterRoleMaster
}

func (s *Sentinel) setSentinelInfo(ctx context.Context, ttl time.Duration) error {
	sentinelInfo := &cluster.SentinelInfo{
		UID: s.uid,
	}
	log.Debugw("sentinelInfo dump", "sentinelInfo", sentinelInfo)

	if err := s.e.SetSentinelInfo(ctx, sentinelInfo, ttl); err != nil {
		return err
	}
	return nil
}

func (s *Sentinel) SetKeeperError(uid string) {
	if _, ok := s.keeperErrorTimers[uid]; !ok {
		s.keeperErrorTimers[uid] = timer.Now()
	}
}

func (s *Sentinel) CleanKeeperError(uid string) {
	delete(s.keeperErrorTimers, uid)
}

func (s *Sentinel) SetDBError(uid string) {
	if _, ok := s.dbErrorTimers[uid]; !ok {
		s.dbErrorTimers[uid] = timer.Now()
	}
}

func (s *Sentinel) CleanDBError(uid string) {
	delete(s.dbErrorTimers, uid)
}

func (s *Sentinel) SetDBNotIncreasingXLogPos(uid string) {
	if _, ok := s.dbNotIncreasingXLogPos[uid]; !ok {
		s.dbNotIncreasingXLogPos[uid] = 1
	} else {
		s.dbNotIncreasingXLogPos[uid] = s.dbNotIncreasingXLogPos[uid] + 1
	}
}

func (s *Sentinel) CleanDBNotIncreasingXLogPos(uid string) {
	delete(s.dbNotIncreasingXLogPos, uid)
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

	// Keepers support several command line arguments that should be populated in the
	// KeeperStatus by the sentinel. This allows us to make decisions about how to arrange
	// the cluster that take into consideration the configuration of each keeper.
	for keeperUID, k := range cd.Keepers {
		if ki, ok := keepersInfo[keeperUID]; ok {
			k.Status.CanBeMaster = ki.CanBeMaster
			k.Status.CanBeSynchronousReplica = ki.CanBeSynchronousReplica
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
			k.Status.PostgresBinaryVersion.Maj = ki.PostgresBinaryVersion.Maj
			k.Status.PostgresBinaryVersion.Min = ki.PostgresBinaryVersion.Min
		}
	}

	// Update keepers' healthy states
	for _, k := range cd.Keepers {
		healthy := s.isKeeperHealthy(cd, k)
		if k.Status.ForceFail {
			healthy = false
			// reset ForceFail
			k.Status.ForceFail = false
		}
		// set zero LastHealthyTime to time.Now() to avoid the keeper being
		// removed since previous versions don't have it set
		if k.Status.LastHealthyTime.IsZero() {
			k.Status.LastHealthyTime = time.Now()
		}
		if healthy {
			k.Status.LastHealthyTime = time.Now()
		}
		k.Status.Healthy = healthy
	}

	// Update dbs' states
	for _, db := range cd.DBs {
		// Mark not found DBs in DBstates in error
		k, ok := keepersInfo[db.Spec.KeeperUID]
		if !ok {
			log.Warnw("no keeper info available", "db", db.UID, "keeper", db.Spec.KeeperUID)
			s.SetDBError(db.UID)
			continue
		}
		dbs := k.PostgresState
		if dbs == nil {
			log.Warnw("no db state available", "db", db.UID, "keeper", db.Spec.KeeperUID)
			s.SetDBError(db.UID)
			continue
		}
		if dbs.UID != db.UID {
			log.Warnw("received db state for unexpected db uid", "receivedDB", dbs.UID, "db", db.UID, "keeper", db.Spec.KeeperUID)
			s.SetDBError(db.UID)
			continue
		}
		log.Debugw("received db state", "db", db.UID, "keeper", db.Spec.KeeperUID)

		if db.Status.XLogPos == dbs.XLogPos {
			s.SetDBNotIncreasingXLogPos(db.UID)
		} else {
			s.CleanDBNotIncreasingXLogPos(db.UID)
		}

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

			db.Status.CurSynchronousStandbys = dbs.SynchronousStandbys

			db.Status.OlderWalFile = dbs.OlderWalFile
		} else {
			s.SetDBError(db.UID)
		}

	}

	// Update dbs' healthy state
	for _, db := range cd.DBs {
		db.Status.Healthy = s.isDBHealthy(cd, db)
		// if keeper is unhealthy then mark also the db ad unhealthy
		keeper := cd.Keepers[db.Spec.KeeperUID]
		if !keeper.Status.Healthy {
			db.Status.Healthy = false
		}
	}

	return cd, kihs
}

// activeProxiesInfos takes the provided proxyInfo list and returns a list of
// proxiesInfo considered active. We also consider as active the proxies not yet
// in the proxyInfoHistories since only after some time we'll know if they are
// really active (updating their proxyInfo) or stale. This is needed to not
// exclude any possible active proxy from the checks in updateCluster and not
// remove them from the enabled proxies list. At worst a stale proxy will be
// added to the enabled proxies list.
func (s *Sentinel) activeProxiesInfos(proxiesInfo cluster.ProxiesInfo) cluster.ProxiesInfo {
	pihs := s.proxyInfoHistories.DeepCopy()

	// remove missing proxyInfos from the history
	for proxyUID := range pihs {
		if _, ok := proxiesInfo[proxyUID]; !ok {
			delete(pihs, proxyUID)
		}

	}

	activeProxiesInfo := proxiesInfo.DeepCopy()
	// keep only updated proxies info
	for _, pi := range proxiesInfo {
		if pih, ok := pihs[pi.UID]; ok {
			if pih.ProxyInfo.InfoUID == pi.InfoUID {
				if timer.Since(pih.Timer) > 2*pi.ProxyTimeout {
					delete(activeProxiesInfo, pi.UID)
				}
			} else {
				pihs[pi.UID] = &ProxyInfoHistory{ProxyInfo: pi, Timer: timer.Now()}
			}
		} else {
			// add proxyInfo if not in the history
			pihs[pi.UID] = &ProxyInfoHistory{ProxyInfo: pi, Timer: timer.Now()}
		}
	}

	s.proxyInfoHistories = pihs

	return activeProxiesInfo
}

func (s *Sentinel) findInitialKeeper(cd *cluster.ClusterData) (*cluster.Keeper, error) {
	if len(cd.Keepers) < 1 {
		return nil, fmt.Errorf("no keepers registered")
	}
	r := s.RandFn(len(cd.Keepers))
	keys := []string{}
	for k := range cd.Keepers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return cd.Keepers[keys[r]], nil
}

// setDBSpecFromClusterSpec updates dbSpec values with the related clusterSpec ones
func (s *Sentinel) setDBSpecFromClusterSpec(cd *cluster.ClusterData) {
	clusterSpec := cd.Cluster.DefSpec()
	for _, db := range cd.DBs {
		db.Spec.RequestTimeout = *clusterSpec.RequestTimeout
		db.Spec.MaxStandbys = *clusterSpec.MaxStandbys
		db.Spec.UsePgrewind = *clusterSpec.UsePgrewind
		db.Spec.PGParameters = clusterSpec.PGParameters
		db.Spec.PGHBA = clusterSpec.PGHBA
		if db.Spec.FollowConfig != nil && db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
			db.Spec.FollowConfig.StandbySettings = clusterSpec.StandbyConfig.StandbySettings
			db.Spec.FollowConfig.ArchiveRecoverySettings = clusterSpec.StandbyConfig.ArchiveRecoverySettings
		}
		db.Spec.AdditionalWalSenders = *clusterSpec.AdditionalWalSenders
		switch s.dbType(cd, db.UID) {
		case dbTypeMaster:
			db.Spec.AdditionalReplicationSlots = clusterSpec.AdditionalMasterReplicationSlots
		case dbTypeStandby:
			db.Spec.AdditionalReplicationSlots = nil
			// TODO(sgotti). Update when there'll be an option to define
			// additional replication slots on standbys
		}
	}
}

func (s *Sentinel) isDifferentTimelineBranch(followedDB *cluster.DB, db *cluster.DB) bool {
	if followedDB.Status.TimelineID < db.Status.TimelineID {
		log.Infow("followed instance timeline < than our timeline", "followedTimeline", followedDB.Status.TimelineID, "timeline", db.Status.TimelineID)
		return true
	}

	// if the timelines are the same check that also the switchpoints are the same.
	if followedDB.Status.TimelineID == db.Status.TimelineID {
		if db.Status.TimelineID <= 1 {
			// if timeline <= 1 then no timeline history file exists.
			return false
		}
		ftlh := followedDB.Status.TimelinesHistory.GetTimelineHistory(db.Status.TimelineID - 1)
		tlh := db.Status.TimelinesHistory.GetTimelineHistory(db.Status.TimelineID - 1)
		if ftlh == nil || tlh == nil {
			// No timeline history to check
			return false
		}
		if ftlh.SwitchPoint == tlh.SwitchPoint {
			return false
		}
		log.Infow("followed instance timeline forked at a different xlog pos than our timeline", "followedTimeline", followedDB.Status.TimelineID, "followedXlogpos", ftlh.SwitchPoint, "timeline", db.Status.TimelineID, "xlogpos", tlh.SwitchPoint)
		return true
	}

	// followedDB.Status.TimelineID > db.Status.TimelineID
	ftlh := followedDB.Status.TimelinesHistory.GetTimelineHistory(db.Status.TimelineID)
	if ftlh != nil {
		if ftlh.SwitchPoint < db.Status.XLogPos {
			log.Infow("followed instance timeline forked before our current state", "followedTimeline", followedDB.Status.TimelineID, "followedXlogpos", ftlh.SwitchPoint, "timeline", db.Status.TimelineID, "xlogpos", db.Status.XLogPos)
			return true
		}
	}
	return false
}

// isLagBelowMax checks if the db reported lag is below MaxStandbyLag from the
// master reported lag
func (s *Sentinel) isLagBelowMax(cd *cluster.ClusterData, curMasterDB, db *cluster.DB) bool {
	log.Debugf("curMasterDB.Status.XLogPos: %d, db.Status.XLogPos: %d, lag: %d", curMasterDB.Status.XLogPos, db.Status.XLogPos, int64(curMasterDB.Status.XLogPos-db.Status.XLogPos))
	if int64(curMasterDB.Status.XLogPos-db.Status.XLogPos) > int64(*cd.Cluster.DefSpec().MaxStandbyLag) {
		log.Infow("ignoring keeper since its behind that maximum xlog position", "db", db.UID, "dbXLogPos", db.Status.XLogPos, "masterXLogPos", curMasterDB.Status.XLogPos)
		return false
	}
	return true
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

type dbType int
type dbValidity int
type dbStatus int

const (
	// TODO(sgotti) change "master" and "standby" to different name to
	// better differentiate with with master and standby db roles.
	dbTypeMaster dbType = iota
	dbTypeStandby

	dbValidityValid dbValidity = iota
	dbValidityInvalid
	dbValidityUnknown

	dbStatusGood dbStatus = iota
	dbStatusFailed
	dbStatusConverging
)

// dbType returns the db type
// A master is a db that:
// * Has a master db role or a standby db role with followtype external
// A standby is a db that:
// * Has a standby db role with followtype internal
func (s *Sentinel) dbType(cd *cluster.ClusterData, dbUID string) dbType {
	db, ok := cd.DBs[dbUID]
	if !ok {
		panic(fmt.Errorf("requested unexisting db uid %q", dbUID))
	}
	switch db.Spec.Role {
	case common.RoleMaster:
		return dbTypeMaster
	case common.RoleStandby:
		if db.Spec.FollowConfig.Type == cluster.FollowTypeExternal {
			return dbTypeMaster
		}
		return dbTypeStandby
	default:
		panic("invalid db type in db Spec")
	}
}

// dbValidity return the validity of a db
// a db isn't valid when it has a different postgres systemdID or is on a
// different timeline branch
// dbs with CurrentGeneration == NoGeneration (0) are reported as
// dbValidityUnknown since the db status is empty.
func (s *Sentinel) dbValidity(cd *cluster.ClusterData, dbUID string) dbValidity {
	db, ok := cd.DBs[dbUID]
	if !ok {
		panic(fmt.Errorf("requested unexisting db uid %q", dbUID))
	}

	if db.Status.CurrentGeneration == cluster.NoGeneration {
		return dbValidityUnknown
	}

	masterDB := cd.DBs[cd.Cluster.Status.Master]

	// ignore empty (not provided) systemid
	if db.Status.SystemID != "" {
		// if with a different postgres systemID it's invalid
		if db.Status.SystemID != masterDB.Status.SystemID {
			log.Infow("invalid db since the postgres systemdID is different that the master one", "db", db.UID, "keeper", db.Spec.KeeperUID, "dbSystemdID", db.Status.SystemID, "masterSystemID", masterDB.Status.SystemID)
			return dbValidityInvalid
		}
	}

	// If on a different timeline branch it's invalid
	if s.isDifferentTimelineBranch(masterDB, db) {
		return dbValidityInvalid
	}

	// db is valid
	return dbValidityValid
}

func (s *Sentinel) dbCanSync(cd *cluster.ClusterData, dbUID string) bool {
	db, ok := cd.DBs[dbUID]
	if !ok {
		panic(fmt.Errorf("requested unexisting db uid %q", dbUID))
	}
	masterDB := cd.DBs[cd.Cluster.Status.Master]

	// ignore if master doesn't provide the older wal file
	if masterDB.Status.OlderWalFile == "" {
		return true
	}

	// skip current master
	if dbUID == masterDB.UID {
		return true
	}

	// skip the standbys
	if s.dbType(cd, db.UID) != dbTypeStandby {
		return true
	}

	// only check when db isn't initializing
	if db.Generation == cluster.InitialGeneration {
		return true
	}

	// check only if the db isn't healty.
	if !db.Status.Healthy {
		return true
	}

	if db.Status.XLogPos == masterDB.Status.XLogPos {
		return true
	}

	// check only if the xlogpos isn't increasing for some time. This can also
	// happen when no writes are happening on the master but the standby should
	// be, if syncing at the same xlogpos.
	if s.isDBIncreasingXLogPos(cd, db) {
		return true
	}

	required := pg.XlogPosToWalFileNameNoTimeline(db.Status.XLogPos)
	older, err := pg.WalFileNameNoTimeLine(masterDB.Status.OlderWalFile)
	if err != nil {
		// warn on wrong file name (shouldn't happen...)
		log.Warnw("wrong wal file name", "filename", masterDB.Status.OlderWalFile)
	}
	log.Debugw("xlog pos isn't advancing on standby, checking if the master has the required wals", "db", db.UID, "keeper", db.Spec.KeeperUID, "requiredWAL", required, "olderMasterWAL", older)
	// compare the required wal file with the older wal file name ignoring the timelineID
	if required >= older {
		return true
	}

	log.Infow("db won't be able to sync due to missing required wals on master", "db", db.UID, "keeper", db.Spec.KeeperUID, "requiredWAL", required, "olderMasterWAL", older)
	return false
}

func (s *Sentinel) dbStatus(cd *cluster.ClusterData, dbUID string) dbStatus {
	db, ok := cd.DBs[dbUID]
	if !ok {
		panic(fmt.Errorf("requested unexisting db uid %q", dbUID))
	}

	// if keeper failed then mark as failed
	keeper := cd.Keepers[db.Spec.KeeperUID]
	if !keeper.Status.Healthy {
		return dbStatusFailed
	}

	convergenceTimeout := cd.Cluster.DefSpec().ConvergenceTimeout.Duration
	// check if db should be in init mode and adjust convergence timeout
	if db.Generation == cluster.InitialGeneration {
		if db.Spec.InitMode == cluster.DBInitModeResync {
			convergenceTimeout = cd.Cluster.DefSpec().SyncTimeout.Duration
		}

	}
	convergenceState := s.dbConvergenceState(db, convergenceTimeout)
	switch convergenceState {
	// if convergence failed then mark as failed
	case ConvergenceFailed:
		return dbStatusFailed
	// if converging then it's not failed (it can also be not healthy since it could be resyncing)
	case Converging:
		return dbStatusConverging
	}
	// if converged but not healthy mark as failed
	if !db.Status.Healthy {
		return dbStatusFailed
	}

	// TODO(sgotti) Check that the standby is successfully syncing with the
	// master (there can be different reasons:
	// * standby cannot connect to the master (network problems)
	// * missing wal segments (this shouldn't happen while keeping the same
	// master since we aren't removing replication slots for the life of a
	// standbydb in the cluster data, but could happen when electing a new
	// master if the elected standby db cluster doesn't have all the wals)

	// db is good
	return dbStatusGood
}

func (s *Sentinel) validMastersByStatus(cd *cluster.ClusterData) (map[string]*cluster.DB, map[string]*cluster.DB, map[string]*cluster.DB) {
	goodMasters := map[string]*cluster.DB{}
	failedMasters := map[string]*cluster.DB{}
	convergingMasters := map[string]*cluster.DB{}

	for _, db := range cd.DBs {
		// keep only valid masters
		if s.dbValidity(cd, db.UID) != dbValidityValid || s.dbType(cd, db.UID) != dbTypeMaster {
			continue
		}
		status := s.dbStatus(cd, db.UID)
		switch status {
		case dbStatusGood:
			goodMasters[db.UID] = db
		case dbStatusFailed:
			failedMasters[db.UID] = db
		case dbStatusConverging:
			convergingMasters[db.UID] = db
		}
	}
	return goodMasters, failedMasters, convergingMasters
}

func (s *Sentinel) validStandbysByStatus(cd *cluster.ClusterData) (map[string]*cluster.DB, map[string]*cluster.DB, map[string]*cluster.DB) {
	goodStandbys := map[string]*cluster.DB{}
	failedStandbys := map[string]*cluster.DB{}
	convergingStandbys := map[string]*cluster.DB{}

	for _, db := range cd.DBs {
		// keep only valid standbys
		if s.dbValidity(cd, db.UID) != dbValidityValid || s.dbType(cd, db.UID) != dbTypeStandby {
			continue
		}
		status := s.dbStatus(cd, db.UID)
		switch status {
		case dbStatusGood:
			goodStandbys[db.UID] = db
		case dbStatusFailed:
			failedStandbys[db.UID] = db
		case dbStatusConverging:
			convergingStandbys[db.UID] = db
		}
	}
	return goodStandbys, failedStandbys, convergingStandbys
}

// dbSlice implements sort interface to sort by XLogPos
type dbSlice []*cluster.DB

func (p dbSlice) Len() int           { return len(p) }
func (p dbSlice) Less(i, j int) bool { return p[i].Status.XLogPos < p[j].Status.XLogPos }
func (p dbSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (s *Sentinel) findBestStandbys(cd *cluster.ClusterData, masterDB *cluster.DB) []*cluster.DB {
	goodStandbys, _, _ := s.validStandbysByStatus(cd)
	bestDBs := []*cluster.DB{}
	for _, db := range goodStandbys {
		if db.Status.TimelineID != masterDB.Status.TimelineID {
			log.Debugw("ignoring keeper since its pg timeline is different than master timeline", "db", db.UID, "dbTimeline", db.Status.TimelineID, "masterTimeline", masterDB.Status.TimelineID)
			continue
		}
		// do this only when not using synchronous replication since in sync repl we
		// have to ignore the last reported xlogpos or valid sync standby will be
		// skipped
		if !s.syncRepl(cd.Cluster.DefSpec()) {
			if !s.isLagBelowMax(cd, masterDB, db) {
				log.Debugw("ignoring keeper since its lag is above the max configured lag", "db", db.UID, "dbXLogPos", db.Status.XLogPos, "masterXLogPos", masterDB.Status.XLogPos)
				continue
			}
		}
		bestDBs = append(bestDBs, db)
	}
	// Sort by XLogPos
	sort.Sort(dbSlice(bestDBs))
	return bestDBs
}

// findBestNewMasters identifies the DBs that are elegible to become a new master. We do
// this by selecting from valid standbys (those keepers that follow the same timeline as
// our master, and have an acceptable replication lag) and also selecting from those nodes
// that are valid to become master by their status.
func (s *Sentinel) findBestNewMasters(cd *cluster.ClusterData, masterDB *cluster.DB) []*cluster.DB {
	bestNewMasters := []*cluster.DB{}
	for _, db := range s.findBestStandbys(cd, masterDB) {
		if k, ok := cd.Keepers[db.Spec.KeeperUID]; ok && (k.Status.CanBeMaster != nil && !*k.Status.CanBeMaster) {
			log.Infow("ignoring keeper since it cannot be master (--can-be-master=false)", "db", db.UID, "keeper", db.Spec.KeeperUID)
			continue
		}

		bestNewMasters = append(bestNewMasters, db)
	}

	// Add the previous masters to the best standbys (if valid and in good state)
	validMastersByStatus, _, _ := s.validMastersByStatus(cd)
	log.Debugf("validMastersByStatus: %s", spew.Sdump(validMastersByStatus))
	for _, db := range validMastersByStatus {
		if db.UID == masterDB.UID {
			log.Debugw("ignoring db since it's the current master", "db", db.UID, "keeper", db.Spec.KeeperUID)
			continue
		}

		if db.Status.TimelineID != masterDB.Status.TimelineID {
			log.Debugw("ignoring keeper since its pg timeline is different than master timeline", "db", db.UID, "dbTimeline", db.Status.TimelineID, "masterTimeline", masterDB.Status.TimelineID)
			continue
		}

		// do this only when not using synchronous replication since in sync repl we
		// have to ignore the last reported xlogpos or valid sync standby will be
		// skipped
		if !s.syncRepl(cd.Cluster.DefSpec()) {
			if !s.isLagBelowMax(cd, masterDB, db) {
				log.Debugw("ignoring keeper since its lag is above the max configured lag", "db", db.UID, "dbXLogPos", db.Status.XLogPos, "masterXLogPos", masterDB.Status.XLogPos)
				continue
			}
		}

		bestNewMasters = append(bestNewMasters, db)
	}

	// Sort by XLogPos
	sort.Sort(dbSlice(bestNewMasters))
	log.Debugf("bestNewMasters: %s", spew.Sdump(bestNewMasters))
	return bestNewMasters
}

func (s *Sentinel) updateCluster(cd *cluster.ClusterData, pis cluster.ProxiesInfo) (*cluster.ClusterData, error) {
	// take a cd deepCopy to check that the code isn't changing it (it'll be a bug)
	origcd := cd.DeepCopy()
	newcd := cd.DeepCopy()
	clusterSpec := cd.Cluster.DefSpec()

	switch cd.Cluster.Status.Phase {
	case cluster.ClusterPhaseInitializing:
		switch *clusterSpec.InitMode {
		case cluster.ClusterInitModeNew:
			// Is there already a keeper choosed to be the new master?
			if cd.Cluster.Status.Master == "" {
				log.Infow("trying to find initial master")
				k, err := s.findInitialKeeper(newcd)
				if err != nil {
					return nil, fmt.Errorf("cannot choose initial master: %v", err)
				}
				log.Infow("initializing cluster", "keeper", k.UID)
				db := &cluster.DB{
					UID:        s.UIDFn(),
					Generation: cluster.InitialGeneration,
					Spec: &cluster.DBSpec{
						KeeperUID:     k.UID,
						InitMode:      cluster.DBInitModeNew,
						NewConfig:     clusterSpec.NewConfig,
						Role:          common.RoleMaster,
						Followers:     []string{},
						IncludeConfig: *clusterSpec.MergePgParameters,
					},
				}
				newcd.DBs[db.UID] = db
				newcd.Cluster.Status.Master = db.UID
				log.Debugf("newcd dump: %s", spew.Sdump(newcd))
			} else {
				db, ok := newcd.DBs[cd.Cluster.Status.Master]
				if !ok {
					panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
				}
				// Check that the choosed db for being the master has correctly initialized
				switch s.dbConvergenceState(db, clusterSpec.InitTimeout.Duration) {
				case Converged:
					if db.Status.Healthy {
						log.Infow("db initialized", "db", db.UID, "keeper", db.Spec.KeeperUID)
						// Set db initMode to none, not needed but just a security measure
						db.Spec.InitMode = cluster.DBInitModeNone
						// Don't include previous config anymore
						db.Spec.IncludeConfig = false

						// Replace reported pg parameters in cluster spec
						if *clusterSpec.MergePgParameters {
							newcd.Cluster.Spec.PGParameters = db.Status.PGParameters
						}
						// Cluster initialized, switch to Normal state
						newcd.Cluster.Status.Phase = cluster.ClusterPhaseNormal
					}
				case Converging:
					log.Infow("waiting for db", "db", db.UID, "keeper", db.Spec.KeeperUID)
				case ConvergenceFailed:
					log.Infow("db failed to initialize", "db", db.UID, "keeper", db.Spec.KeeperUID)
					// Empty DBs
					newcd.DBs = cluster.DBs{}
					// Unset master so another keeper can be chosen
					newcd.Cluster.Status.Master = ""
				}
			}
		case cluster.ClusterInitModeExisting:
			if cd.Cluster.Status.Master == "" {
				wantedKeeper := clusterSpec.ExistingConfig.KeeperUID
				log.Infow("trying to use keeper as initial master", "keeper", wantedKeeper)

				k, ok := newcd.Keepers[wantedKeeper]
				if !ok {
					return nil, fmt.Errorf("keeper %q state not available", wantedKeeper)
				}

				log.Infow("initializing cluster using selected keeper as master db owner", "keeper", k.UID)

				db := &cluster.DB{
					UID:        s.UIDFn(),
					Generation: cluster.InitialGeneration,
					Spec: &cluster.DBSpec{
						KeeperUID:     k.UID,
						InitMode:      cluster.DBInitModeExisting,
						Role:          common.RoleMaster,
						Followers:     []string{},
						IncludeConfig: *clusterSpec.MergePgParameters,
					},
				}
				newcd.DBs[db.UID] = db
				newcd.Cluster.Status.Master = db.UID
				log.Debugf("newcd dump: %s", spew.Sdump(newcd))
			} else {
				db, ok := newcd.DBs[cd.Cluster.Status.Master]
				if !ok {
					panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
				}
				// Check that the choosed db for being the master has correctly initialized
				if db.Status.Healthy && s.dbConvergenceState(db, clusterSpec.ConvergenceTimeout.Duration) == Converged {
					log.Infow("db initialized", "db", db.UID, "keeper", db.Spec.KeeperUID)
					// Set db initMode to none, not needed but just a security measure
					db.Spec.InitMode = cluster.DBInitModeNone
					// Don't include previous config anymore
					db.Spec.IncludeConfig = false
					// Replace reported pg parameters in cluster spec
					if *clusterSpec.MergePgParameters {
						newcd.Cluster.Spec.PGParameters = db.Status.PGParameters
					}
					// Cluster initialized, switch to Normal state
					newcd.Cluster.Status.Phase = cluster.ClusterPhaseNormal
				}
			}
		case cluster.ClusterInitModePITR:
			// Is there already a keeper choosed to be the new master?
			if cd.Cluster.Status.Master == "" {
				log.Infow("trying to find initial master")
				k, err := s.findInitialKeeper(cd)
				if err != nil {
					return nil, fmt.Errorf("cannot choose initial master: %v", err)
				}
				log.Infow("initializing cluster using selected keeper as master db owner", "keeper", k.UID)
				role := common.RoleMaster
				var followConfig *cluster.FollowConfig
				if *clusterSpec.Role == cluster.ClusterRoleStandby {
					role = common.RoleStandby
					followConfig = &cluster.FollowConfig{
						Type:                    cluster.FollowTypeExternal,
						StandbySettings:         clusterSpec.StandbyConfig.StandbySettings,
						ArchiveRecoverySettings: clusterSpec.StandbyConfig.ArchiveRecoverySettings,
					}
				}
				db := &cluster.DB{
					UID:        s.UIDFn(),
					Generation: cluster.InitialGeneration,
					Spec: &cluster.DBSpec{
						KeeperUID:     k.UID,
						InitMode:      cluster.DBInitModePITR,
						PITRConfig:    clusterSpec.PITRConfig,
						Role:          role,
						FollowConfig:  followConfig,
						Followers:     []string{},
						IncludeConfig: *clusterSpec.MergePgParameters,
					},
				}
				newcd.DBs[db.UID] = db
				newcd.Cluster.Status.Master = db.UID
				log.Debugf("newcd dump: %s", spew.Sdump(newcd))
			} else {
				db, ok := newcd.DBs[cd.Cluster.Status.Master]
				if !ok {
					panic(fmt.Errorf("db %q object doesn't exists. This shouldn't happen", cd.Cluster.Status.Master))
				}
				// Check that the choosed db for being the master has correctly initialized
				// TODO(sgotti) set a timeout (the max time for a restore operation)
				switch s.dbConvergenceState(db, 0) {
				case Converged:
					if db.Status.Healthy {
						log.Infow("db initialized", "db", db.UID, "keeper", db.Spec.KeeperUID)
						// Set db initMode to none, not needed but just a security measure
						db.Spec.InitMode = cluster.DBInitModeNone
						// Don't include previous config anymore
						db.Spec.IncludeConfig = false

						// Replace reported pg parameters in cluster spec
						if *clusterSpec.MergePgParameters {
							newcd.Cluster.Spec.PGParameters = db.Status.PGParameters
						}
						// Cluster initialized, switch to Normal state
						newcd.Cluster.Status.Phase = cluster.ClusterPhaseNormal
					}
				case Converging:
					log.Infow("waiting for db to converge", "db", db.UID, "keeper", db.Spec.KeeperUID)
				case ConvergenceFailed:
					log.Infow("db failed to initialize", "db", db.UID, "keeper", db.Spec.KeeperUID)
					// Empty DBs
					newcd.DBs = cluster.DBs{}
					// Unset master so another keeper can be chosen
					newcd.Cluster.Status.Master = ""
				}
			}
		default:
			return nil, fmt.Errorf("unknown init mode %s", *cd.Cluster.DefSpec().InitMode)
		}

	case cluster.ClusterPhaseNormal:
		// Remove old keepers
		keepersToRemove := []*cluster.Keeper{}
		for _, k := range newcd.Keepers {
			// get db associated to the keeper
			db := cd.FindDB(k)
			if db != nil {
				// skip keepers with an assigned db
				continue
			}
			if time.Now().After(k.Status.LastHealthyTime.Add(cd.Cluster.DefSpec().DeadKeeperRemovalInterval.Duration)) {
				log.Infow("removing old dead keeper", "keeper", k.UID)
				keepersToRemove = append(keepersToRemove, k)
			}
		}
		for _, k := range keepersToRemove {
			delete(newcd.Keepers, k.UID)
		}

		// Calculate current master status
		curMasterDBUID := cd.Cluster.Status.Master
		wantedMasterDBUID := curMasterDBUID

		masterOK := true
		curMasterDB := cd.DBs[curMasterDBUID]
		if curMasterDB == nil {
			return nil, fmt.Errorf("db for keeper %q not available. This shouldn't happen!", curMasterDBUID)
		}
		log.Debugf("db dump: %s", spew.Sdump(curMasterDB))

		if !curMasterDB.Status.Healthy {
			log.Infow("master db is failed", "db", curMasterDB.UID, "keeper", curMasterDB.Spec.KeeperUID)
			masterOK = false
		}

		// Check that the wanted master is in master state (i.e. check that promotion from standby to master happened)
		if s.dbConvergenceState(curMasterDB, clusterSpec.ConvergenceTimeout.Duration) == ConvergenceFailed {
			log.Infow("db not converged", "db", curMasterDB.UID, "keeper", curMasterDB.Spec.KeeperUID)
			masterOK = false
		}

		if !masterOK {
			log.Infow("trying to find a new master to replace failed master")
			bestNewMasters := s.findBestNewMasters(newcd, curMasterDB)
			if len(bestNewMasters) == 0 {
				log.Errorw("no eligible masters")
			} else {
				// if synchronous replication is enabled, only choose new master in the synchronous replication standbys.
				var bestNewMasterDB *cluster.DB
				if curMasterDB.Spec.SynchronousReplication {
					commonSyncStandbys := util.CommonElements(curMasterDB.Status.SynchronousStandbys, curMasterDB.Spec.SynchronousStandbys)
					if len(commonSyncStandbys) == 0 {
						log.Warnw("cannot choose synchronous standby since there are no common elements between the latest master reported synchronous standbys and the db spec ones", "reported", curMasterDB.Status.SynchronousStandbys, "spec", curMasterDB.Spec.SynchronousStandbys)
					} else {
						for _, nm := range bestNewMasters {
							if util.StringInSlice(commonSyncStandbys, nm.UID) {
								bestNewMasterDB = nm
								break
							}
						}
						if bestNewMasterDB == nil {
							log.Warnw("cannot choose synchronous standby since there's not match between the possible masters and the usable synchronousStandbys", "reported", curMasterDB.Status.SynchronousStandbys, "spec", curMasterDB.Spec.SynchronousStandbys, "common", commonSyncStandbys, "possibleMasters", bestNewMasters)
						}
					}
				} else {
					bestNewMasterDB = bestNewMasters[0]
				}
				if bestNewMasterDB != nil {
					log.Infow("electing db as the new master", "db", bestNewMasterDB.UID, "keeper", bestNewMasterDB.Spec.KeeperUID)
					wantedMasterDBUID = bestNewMasterDB.UID
				} else {
					log.Errorw("no eligible masters")
				}
			}
		}

		// New master elected
		if curMasterDBUID != wantedMasterDBUID {
			// maintain the current role of the old master and just remove followers
			oldMasterdb := newcd.DBs[curMasterDBUID]
			oldMasterdb.Spec.Followers = []string{}

			masterDBRole := common.RoleMaster
			var followConfig *cluster.FollowConfig
			if *clusterSpec.Role == cluster.ClusterRoleStandby {
				masterDBRole = common.RoleStandby
				followConfig = &cluster.FollowConfig{
					Type:                    cluster.FollowTypeExternal,
					StandbySettings:         clusterSpec.StandbyConfig.StandbySettings,
					ArchiveRecoverySettings: clusterSpec.StandbyConfig.ArchiveRecoverySettings,
				}
			}

			newcd.Cluster.Status.Master = wantedMasterDBUID
			newMasterDB := newcd.DBs[wantedMasterDBUID]
			newMasterDB.Spec.Role = masterDBRole
			newMasterDB.Spec.FollowConfig = followConfig

			// Tell proxy that there's currently no active master
			if newcd.Proxy.Spec.MasterDBUID != "" {
				// Tell proxies that there's currently no active master
				newcd.Proxy.Spec.MasterDBUID = ""
				newcd.Proxy.Generation++
			}

			// Setup synchronous standbys to the one of the previous master (replacing ourself with the previous master)
			if s.syncRepl(clusterSpec) {
				newMasterDB.Spec.SynchronousReplication = true
				newMasterDB.Spec.SynchronousStandbys = []string{}
				newMasterDB.Spec.ExternalSynchronousStandbys = []string{}
				for _, dbUID := range oldMasterdb.Spec.SynchronousStandbys {
					if dbUID != newMasterDB.UID {
						newMasterDB.Spec.SynchronousStandbys = append(newMasterDB.Spec.SynchronousStandbys, dbUID)
					} else {
						newMasterDB.Spec.SynchronousStandbys = append(newMasterDB.Spec.SynchronousStandbys, oldMasterdb.UID)
					}
				}
				if len(newMasterDB.Spec.SynchronousStandbys) == 0 {
					newMasterDB.Spec.ExternalSynchronousStandbys = []string{fakeStandbyName}
				}

				// Just sort to always have them in the same order and avoid
				// unneeded updates to synchronous_standby_names by the keeper.
				sort.Strings(newMasterDB.Spec.SynchronousStandbys)
				sort.Strings(newMasterDB.Spec.ExternalSynchronousStandbys)
			} else {
				newMasterDB.Spec.SynchronousReplication = false
				newMasterDB.Spec.SynchronousStandbys = nil
				newMasterDB.Spec.ExternalSynchronousStandbys = nil
			}
		}

		if curMasterDBUID == wantedMasterDBUID {
			masterDB := newcd.DBs[curMasterDBUID]
			masterDBKeeper := newcd.Keepers[masterDB.Spec.KeeperUID]

			if newcd.Proxy.Spec.MasterDBUID == "" {
				// if the Proxy.Spec.MasterDBUID is empty we have to wait for all
				// the proxies to have converged to be sure they closed connections
				// to previous master or disappear (in this case we assume that they
				// have closed connections to previous master)
				unconvergedProxiesUIDs := []string{}
				for _, pi := range pis {
					if pi.Generation != newcd.Proxy.Generation {
						unconvergedProxiesUIDs = append(unconvergedProxiesUIDs, pi.UID)
					}
				}
				if len(unconvergedProxiesUIDs) > 0 {
					log.Infow("waiting for proxies to be converged to the current generation", "proxies", unconvergedProxiesUIDs)
				} else {
					// Tell proxy that there's a new active master
					newcd.Proxy.Spec.MasterDBUID = wantedMasterDBUID
					newcd.Proxy.Generation++
				}
			} else {
				// if we have Proxy.Spec.MasterDBUID != "" then we have waited for
				// proxies to have converged and we can set enabled proxies to
				// the currently available proxies in proxyInfo.
				enabledProxies := []string{}
				for _, pi := range pis {
					enabledProxies = append(enabledProxies, pi.UID)
				}
				sort.Strings(enabledProxies)
				if !reflect.DeepEqual(newcd.Proxy.Spec.EnabledProxies, enabledProxies) {
					newcd.Proxy.Spec.EnabledProxies = enabledProxies
					newcd.Proxy.Generation++
				}
			}

			// change master db role to "master" if the cluster role has been changed in the spec
			if *clusterSpec.Role == cluster.ClusterRoleMaster {
				masterDB.Spec.Role = common.RoleMaster
				masterDB.Spec.FollowConfig = nil
			}

			// Set standbys to follow master only if it's healthy and converged
			if masterDB.Status.Healthy && s.dbConvergenceState(masterDB, clusterSpec.ConvergenceTimeout.Duration) == Converged {

				// Remove old masters
				toRemove := []*cluster.DB{}
				for _, db := range newcd.DBs {
					if db.UID == wantedMasterDBUID {
						continue
					}
					if s.dbType(newcd, db.UID) != dbTypeMaster {
						continue
					}
					log.Infow("removing old master db", "db", db.UID, "keeper", db.Spec.KeeperUID)
					toRemove = append(toRemove, db)
				}
				for _, db := range toRemove {
					delete(newcd.DBs, db.UID)
				}

				// Remove invalid dbs
				toRemove = []*cluster.DB{}
				for _, db := range newcd.DBs {
					if db.UID == wantedMasterDBUID {
						continue
					}
					if s.dbValidity(newcd, db.UID) != dbValidityInvalid {
						continue
					}
					log.Infow("removing invalid db", "db", db.UID, "keeper", db.Spec.KeeperUID)
					toRemove = append(toRemove, db)
				}
				for _, db := range toRemove {
					delete(newcd.DBs, db.UID)
				}

				// Remove dbs that won't sync due to missing wals on current master
				toRemove = []*cluster.DB{}
				for _, db := range newcd.DBs {
					if s.dbCanSync(cd, db.UID) {
						continue
					}
					log.Infow("removing db that won't be able to sync due to missing wals on current master", "db", db.UID, "keeper", db.Spec.KeeperUID)
					toRemove = append(toRemove, db)
				}
				for _, db := range toRemove {
					delete(newcd.DBs, db.UID)
				}

				goodStandbys, failedStandbys, convergingStandbys := s.validStandbysByStatus(newcd)
				goodStandbysCount := len(goodStandbys)
				failedStandbysCount := len(failedStandbys)
				convergingStandbysCount := len(convergingStandbys)
				log.Debugw("standbys states", "good", goodStandbysCount, "failed", failedStandbysCount, "converging", convergingStandbysCount)

				// Clean InitMode for goodStandbys
				for _, db := range goodStandbys {
					db.Spec.InitMode = cluster.DBInitModeNone
				}

				// Setup synchronous standbys
				if s.syncRepl(clusterSpec) {
					minSynchronousStandbys := int(*clusterSpec.MinSynchronousStandbys)
					maxSynchronousStandbys := int(*clusterSpec.MaxSynchronousStandbys)
					merge := true
					// PostgresSQL <= 9.5 only supports one sync standby at a
					// time (defining multiple sync standbys is like doing "1
					// (standby1, standby2)" on postgres >= 9.6 and so we won't
					// be able to know which is the real in sync standby.
					//
					// So we always have to define 1 standby in
					// masterDB.Spec.SynchronousStandbys with the downside that
					// can there be a time window where we cannot elect the
					// synchronous standby as a new primary if it's not yet in
					// sync
					if masterDBKeeper.Status.PostgresBinaryVersion.Maj != 0 {
						if masterDBKeeper.Status.PostgresBinaryVersion.Maj == 9 && masterDBKeeper.Status.PostgresBinaryVersion.Min <= 5 {
							minSynchronousStandbys = 1
							maxSynchronousStandbys = 1
							merge = false
						}
					}

					// if the current known in sync syncstandbys are different than the required ones wait for them and remove non good ones
					if !util.CompareStringSliceNoOrder(masterDB.Status.SynchronousStandbys, masterDB.Spec.SynchronousStandbys) {

						// remove old syncstandbys from current status
						masterDB.Status.SynchronousStandbys = util.CommonElements(masterDB.Status.SynchronousStandbys, masterDB.Spec.SynchronousStandbys)

						// add reported in sync syncstandbys to the current status
						curSyncStandbys := util.CommonElements(masterDB.Status.CurSynchronousStandbys, masterDB.Spec.SynchronousStandbys)
						toAddSyncStandbys := util.Difference(curSyncStandbys, masterDB.Status.SynchronousStandbys)
						masterDB.Status.SynchronousStandbys = append(masterDB.Status.SynchronousStandbys, toAddSyncStandbys...)

						// if some of the non yet in sync syncstandbys are failed, set Spec.SynchronousStandbys to the current in sync ones, se other could be added.
						notInSyncSyncStandbys := util.Difference(masterDB.Spec.SynchronousStandbys, masterDB.Status.SynchronousStandbys)
						update := false
						for _, dbUID := range notInSyncSyncStandbys {
							if _, ok := newcd.DBs[dbUID]; !ok {
								log.Infow("one of the new synchronousStandbys has been removed", "db", dbUID, "inSyncStandbys", masterDB.Status.SynchronousStandbys, "synchronousStandbys", masterDB.Spec.SynchronousStandbys)
								update = true
								continue
							}
							if _, ok := goodStandbys[dbUID]; !ok {
								log.Infow("one of the new synchronousStandbys is not in good state", "db", dbUID, "inSyncStandbys", masterDB.Status.SynchronousStandbys, "synchronousStandbys", masterDB.Spec.SynchronousStandbys)
								update = true
								continue
							}
						}
						if update {
							// Use the current known in sync syncStandbys as Spec.SynchronousStandbys
							log.Infow("setting the expected sync-standbys to the current known in sync sync-standbys", "inSyncStandbys", masterDB.Status.SynchronousStandbys, "synchronousStandbys", masterDB.Spec.SynchronousStandbys)
							masterDB.Spec.SynchronousStandbys = masterDB.Status.SynchronousStandbys

							// Just sort to always have them in the same order and avoid
							// unneeded updates to synchronous_standby_names by the keeper.
							sort.Strings(masterDB.Spec.SynchronousStandbys)
						}
					}

					// update synchronousStandbys only if the reported
					// SynchronousStandbys are the same as the required ones. In
					// this way, when we have to choose a new master we are sure
					// that there're no intermediate changes between the
					// reported standbys and the required ones.
					if !util.CompareStringSliceNoOrder(masterDB.Status.SynchronousStandbys, masterDB.Spec.SynchronousStandbys) {
						log.Infow("waiting for new defined synchronous standbys to be in sync", "inSyncStandbys", curMasterDB.Status.SynchronousStandbys, "synchronousStandbys", curMasterDB.Spec.SynchronousStandbys)
					} else {
						addFakeStandby := false
						externalSynchronousStandbys := map[string]struct{}{}

						// make a map of synchronous standbys starting from the current ones
						prevSynchronousStandbys := map[string]struct{}{}
						synchronousStandbys := map[string]struct{}{}

						for _, dbUID := range masterDB.Spec.SynchronousStandbys {
							prevSynchronousStandbys[dbUID] = struct{}{}
							synchronousStandbys[dbUID] = struct{}{}
						}

						// Remove not existing dbs (removed above)
						toRemove := map[string]struct{}{}
						for dbUID := range synchronousStandbys {
							if _, ok := newcd.DBs[dbUID]; !ok {
								log.Infow("removing non existent db from synchronousStandbys", "masterDB", masterDB.UID, "db", dbUID)
								toRemove[dbUID] = struct{}{}
							}
						}
						for dbUID := range toRemove {
							delete(synchronousStandbys, dbUID)
						}

						// Check if the current synchronous standbys are healthy or remove them
						toRemove = map[string]struct{}{}
						for dbUID := range synchronousStandbys {
							if _, ok := goodStandbys[dbUID]; !ok {
								log.Infow("removing failed synchronous standby", "masterDB", masterDB.UID, "db", dbUID)
								toRemove[dbUID] = struct{}{}
							}
						}
						for dbUID := range toRemove {
							delete(synchronousStandbys, dbUID)
						}

						// Remove synchronous standbys in excess
						if len(synchronousStandbys) > maxSynchronousStandbys {
							rc := len(synchronousStandbys) - maxSynchronousStandbys
							removedCount := 0
							toRemove = map[string]struct{}{}
							for dbUID := range synchronousStandbys {
								if removedCount >= rc {
									break
								}
								log.Infow("removing synchronous standby in excess", "masterDB", masterDB.UID, "db", dbUID)
								toRemove[dbUID] = struct{}{}
								removedCount++
							}
							for dbUID := range toRemove {
								delete(synchronousStandbys, dbUID)
							}
						}

						// try to add missing standbys up to MaxSynchronousStandbys
						bestStandbys := s.findBestStandbys(newcd, curMasterDB)

						ac := maxSynchronousStandbys - len(synchronousStandbys)
						addedCount := 0
						for _, bestStandby := range bestStandbys {
							if addedCount >= ac {
								break
							}
							if _, ok := synchronousStandbys[bestStandby.UID]; ok {
								continue
							}

							// ignore standbys that cannot be synchronous standbys
							if db, ok := newcd.DBs[bestStandby.UID]; ok {
								if keeper, ok := newcd.Keepers[db.Spec.KeeperUID]; ok && (keeper.Status.CanBeSynchronousReplica != nil && !*keeper.Status.CanBeSynchronousReplica) {
									log.Infow("cannot choose standby as synchronous (--can-be-synchronous-replica=false)", "db", db.UID, "keeper", keeper.UID)
									continue
								}
							}

							log.Infow("adding new synchronous standby in good state trying to reach MaxSynchronousStandbys", "masterDB", masterDB.UID, "synchronousStandbyDB", bestStandby.UID, "keeper", bestStandby.Spec.KeeperUID)
							synchronousStandbys[bestStandby.UID] = struct{}{}
							addedCount++
						}

						// If there're some missing standbys to reach
						// MinSynchronousStandbys, keep previous sync standbys,
						// also if not in a good state. In this way we have more
						// possibilities to choose a sync standby to replace a
						// failed master if they becoe healthy again
						ac = minSynchronousStandbys - len(synchronousStandbys)
						addedCount = 0
						for _, db := range newcd.DBs {
							if addedCount >= ac {
								break
							}
							if _, ok := synchronousStandbys[db.UID]; ok {
								continue
							}
							if _, ok := prevSynchronousStandbys[db.UID]; ok {
								log.Infow("adding previous synchronous standby to reach MinSynchronousStandbys", "masterDB", masterDB.UID, "synchronousStandbyDB", db.UID, "keeper", db.Spec.KeeperUID)
								synchronousStandbys[db.UID] = struct{}{}
								addedCount++
							}
						}

						if merge {
							// if some of the new synchronousStandbys are not inside
							// the prevSynchronousStandbys then also add all
							// the prevSynchronousStandbys. In this way when there's
							// a synchronousStandbys change we'll have, in a first
							// step, both the old and the new standbys, then in the
							// second step the old will be removed (since the new
							// standbys are all inside prevSynchronousStandbys), so
							// we'll always be able to choose a sync standby that we
							// know was defined in the primary and in sync if the
							// primary fails.
							allInPrev := true
							for k := range synchronousStandbys {
								if _, ok := prevSynchronousStandbys[k]; !ok {
									allInPrev = false
								}
							}
							if !allInPrev {
								log.Infow("merging current and previous synchronous standbys", "masterDB", masterDB.UID, "prevSynchronousStandbys", prevSynchronousStandbys, "synchronousStandbys", synchronousStandbys)
								// use only existing dbs
								for _, db := range newcd.DBs {
									if _, ok := prevSynchronousStandbys[db.UID]; ok {
										log.Infow("adding previous synchronous standby", "masterDB", masterDB.UID, "synchronousStandbyDB", db.UID, "keeper", db.Spec.KeeperUID)
										synchronousStandbys[db.UID] = struct{}{}
									}
								}
							}
						}

						if !reflect.DeepEqual(synchronousStandbys, prevSynchronousStandbys) {
							log.Infow("synchronousStandbys changed", "masterDB", masterDB.UID, "prevSynchronousStandbys", prevSynchronousStandbys, "synchronousStandbys", synchronousStandbys)
						} else {
							log.Debugf("synchronousStandbys not changed", "masterDB", masterDB.UID, "prevSynchronousStandbys", prevSynchronousStandbys, "synchronousStandbys", synchronousStandbys)
						}

						// If there're not enough real synchronous standbys add a fake synchronous standby because we have to be strict and make the master block transactions until MinSynchronousStandbys real standbys are available
						if len(synchronousStandbys)+len(externalSynchronousStandbys) < minSynchronousStandbys {
							log.Infow("using a fake synchronous standby since there are not enough real standbys available", "masterDB", masterDB.UID, "required", minSynchronousStandbys)
							addFakeStandby = true
						}

						masterDB.Spec.SynchronousReplication = true
						masterDB.Spec.SynchronousStandbys = []string{}
						masterDB.Spec.ExternalSynchronousStandbys = []string{}
						for dbUID := range synchronousStandbys {
							masterDB.Spec.SynchronousStandbys = append(masterDB.Spec.SynchronousStandbys, dbUID)
						}

						for dbUID := range externalSynchronousStandbys {
							masterDB.Spec.ExternalSynchronousStandbys = append(masterDB.Spec.ExternalSynchronousStandbys, dbUID)
						}

						if addFakeStandby {
							masterDB.Spec.ExternalSynchronousStandbys = append(masterDB.Spec.ExternalSynchronousStandbys, fakeStandbyName)
						}

						// remove old syncstandbys from current status
						masterDB.Status.SynchronousStandbys = util.CommonElements(masterDB.Status.SynchronousStandbys, masterDB.Spec.SynchronousStandbys)

						// Just sort to always have them in the same order and avoid
						// unneeded updates to synchronous_standby_names by the keeper.
						sort.Strings(masterDB.Spec.SynchronousStandbys)
						sort.Strings(masterDB.Spec.ExternalSynchronousStandbys)
					}
				} else {
					masterDB.Spec.SynchronousReplication = false
					masterDB.Spec.SynchronousStandbys = nil
					masterDB.Spec.ExternalSynchronousStandbys = nil

					masterDB.Status.SynchronousStandbys = nil
				}

				// NotFailed != Good since there can be some dbs that are converging
				// it's the total number of standbys - the failed standbys
				// or the sum of good + converging standbys
				notFailedStandbysCount := goodStandbysCount + convergingStandbysCount

				// Remove dbs in excess if we have a good number >= MaxStandbysPerSender
				// We don't remove failed db until the number of good db is >= MaxStandbysPerSender since they can come back
				if uint16(goodStandbysCount) >= *clusterSpec.MaxStandbysPerSender {
					toRemove := []*cluster.DB{}
					// Remove all non good standbys
					for _, db := range newcd.DBs {
						if s.dbType(newcd, db.UID) != dbTypeStandby {
							continue
						}
						// Don't remove standbys marked as synchronous standbys
						if util.StringInSlice(masterDB.Spec.SynchronousStandbys, db.UID) {
							continue
						}
						if _, ok := goodStandbys[db.UID]; !ok {
							log.Infow("removing non good standby", "db", db.UID)
							toRemove = append(toRemove, db)
						}
					}
					// Remove good standbys in excess
					nr := int(uint16(goodStandbysCount) - *clusterSpec.MaxStandbysPerSender)
					i := 0
					for _, db := range goodStandbys {
						if i >= nr {
							break
						}
						// Don't remove standbys marked as synchronous standbys
						if util.StringInSlice(masterDB.Spec.SynchronousStandbys, db.UID) {
							continue
						}
						log.Infow("removing good standby in excess", "db", db.UID)
						toRemove = append(toRemove, db)
						i++
					}
					for _, db := range toRemove {
						delete(newcd.DBs, db.UID)
					}

				} else {
					// Add new dbs to substitute failed dbs, if there're available keepers.

					// nc can be negative if MaxStandbysPerSender has been lowered
					nc := int(*clusterSpec.MaxStandbysPerSender - uint16(notFailedStandbysCount))
					// Add missing DBs until MaxStandbysPerSender
					freeKeepers := s.freeKeepers(newcd)
					nf := len(freeKeepers)
					for i := 0; i < nc && i < nf; i++ {
						freeKeeper := freeKeepers[i]
						db := &cluster.DB{
							UID:        s.UIDFn(),
							Generation: cluster.InitialGeneration,
							Spec: &cluster.DBSpec{
								KeeperUID:    freeKeeper.UID,
								InitMode:     cluster.DBInitModeResync,
								Role:         common.RoleStandby,
								Followers:    []string{},
								FollowConfig: &cluster.FollowConfig{Type: cluster.FollowTypeInternal, DBUID: wantedMasterDBUID},
							},
						}
						newcd.DBs[db.UID] = db
						log.Infow("added new standby db", "db", db.UID, "keeper", db.Spec.KeeperUID)
					}
				}

				// Reconfigure all standbys as followers of the current master
				for _, db := range newcd.DBs {
					if s.dbType(newcd, db.UID) != dbTypeStandby {
						continue
					}

					db.Spec.Role = common.RoleStandby
					// Remove followers
					db.Spec.Followers = []string{}
					db.Spec.FollowConfig = &cluster.FollowConfig{Type: cluster.FollowTypeInternal, DBUID: wantedMasterDBUID}

					db.Spec.SynchronousReplication = false
					db.Spec.SynchronousStandbys = nil
					db.Spec.ExternalSynchronousStandbys = nil
				}
			}
		}

		// Update followers for master DB
		// Always do this since, in future, keepers and related db could be
		// removed (currently only dead keepers without an assigned db are
		// removed)
		masterDB := newcd.DBs[curMasterDBUID]
		masterDB.Spec.Followers = []string{}
		for _, db := range newcd.DBs {
			if masterDB.UID == db.UID {
				continue
			}
			fc := db.Spec.FollowConfig
			if fc != nil {
				if fc.Type == cluster.FollowTypeInternal && fc.DBUID == wantedMasterDBUID {
					masterDB.Spec.Followers = append(masterDB.Spec.Followers, db.UID)
				}
			}
		}
		// Sort followers so the slice won't be considered changed due to different order of the same entries.
		sort.Strings(masterDB.Spec.Followers)

	default:
		return nil, fmt.Errorf("unknown cluster phase %s", cd.Cluster.Status.Phase)
	}

	// Copy the clusterSpec parameters to the dbSpec
	s.setDBSpecFromClusterSpec(newcd)

	// Update generation on DBs if they have changed
	for dbUID, db := range newcd.DBs {
		prevDB, ok := cd.DBs[dbUID]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(db.Spec, prevDB.Spec) {
			log.Debugw("db spec changed, updating generation", "prevDB", spew.Sdump(prevDB.Spec), "db", spew.Sdump(db.Spec))
			db.Generation++
		}
	}

	// check that we haven't changed the current cd or there's a bug somewhere
	if !reflect.DeepEqual(origcd, cd) {
		return nil, fmt.Errorf("cd was changed in updateCluster, this shouldn't happen!")
	}
	return newcd, nil
}

func (s *Sentinel) updateChangeTimes(cd, newcd *cluster.ClusterData) {
	newcd.ChangeTime = time.Now()

	if !reflect.DeepEqual(newcd.Cluster, cd.Cluster) {
		newcd.Cluster.ChangeTime = time.Now()
	}

	for dbUID, db := range newcd.DBs {
		prevDB, ok := cd.DBs[dbUID]
		if !ok {
			db.ChangeTime = time.Now()
			continue
		}
		if !reflect.DeepEqual(db, prevDB) {
			db.ChangeTime = time.Now()
		}
	}

	for keeperUID, keeper := range newcd.Keepers {
		prevKeeper, ok := cd.Keepers[keeperUID]
		if !ok {
			keeper.ChangeTime = time.Now()
			continue
		}
		if !reflect.DeepEqual(keeper, prevKeeper) {
			keeper.ChangeTime = time.Now()
		}
	}

	if !reflect.DeepEqual(newcd.Proxy, cd.Proxy) {
		newcd.Proxy.ChangeTime = time.Now()
	}
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
	if timer.Since(t) > cd.Cluster.DefSpec().FailInterval.Duration {
		return false
	}
	return true
}

func (s *Sentinel) isDBHealthy(cd *cluster.ClusterData, db *cluster.DB) bool {
	t, ok := s.dbErrorTimers[db.UID]
	if !ok {
		return true
	}
	if timer.Since(t) > cd.Cluster.DefSpec().FailInterval.Duration {
		return false
	}
	return true
}

func (s *Sentinel) isDBIncreasingXLogPos(cd *cluster.ClusterData, db *cluster.DB) bool {
	t, ok := s.dbNotIncreasingXLogPos[db.UID]
	if !ok {
		return true
	}
	if t > cluster.DefaultDBNotIncreasingXLogPosTimes {
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
	if !reflect.DeepEqual(k, nk) {
		panic("not equal")
	}
	return nk.(KeeperInfoHistories)
}

type DBConvergenceInfo struct {
	Generation int64
	Timer      int64
}

type ProxyInfoHistory struct {
	ProxyInfo *cluster.ProxyInfo
	Timer     int64
}

type ProxyInfoHistories map[string]*ProxyInfoHistory

func (p ProxyInfoHistories) DeepCopy() ProxyInfoHistories {
	if p == nil {
		return nil
	}
	np, err := copystructure.Copy(p)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(p, np) {
		panic("not equal")
	}
	return np.(ProxyInfoHistories)
}

type Sentinel struct {
	uid string
	cfg *config
	e   store.Store

	election store.Election
	end      chan bool

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

	keeperErrorTimers      map[string]int64
	dbErrorTimers          map[string]int64
	dbNotIncreasingXLogPos map[string]int64
	dbConvergenceInfos     map[string]*DBConvergenceInfo

	keeperInfoHistories KeeperInfoHistories
	proxyInfoHistories  ProxyInfoHistories
}

func NewSentinel(uid string, cfg *config, end chan bool) (*Sentinel, error) {
	var initialClusterSpec *cluster.ClusterSpec
	if cfg.initialClusterSpecFile != "" {
		configData, err := ioutil.ReadFile(cfg.initialClusterSpecFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read provided initial cluster config file: %v", err)
		}
		if err := json.Unmarshal(configData, &initialClusterSpec); err != nil {
			return nil, fmt.Errorf("cannot parse provided initial cluster config: %v", err)
		}
		log.Debugw("initialClusterSpec dump", "initialClusterSpec", spew.Sdump(initialClusterSpec))
		if err := initialClusterSpec.Validate(); err != nil {
			return nil, fmt.Errorf("invalid initial cluster: %v", err)
		}
	}

	e, err := cmd.NewStore(&cfg.CommonConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	election, err := cmd.NewElection(&cfg.CommonConfig, uid)
	if err != nil {
		return nil, fmt.Errorf("cannot create election: %v", err)
	}

	return &Sentinel{
		uid:                uid,
		cfg:                cfg,
		e:                  e,
		election:           election,
		leader:             false,
		initialClusterSpec: initialClusterSpec,
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

func (s *Sentinel) Start(ctx context.Context) {
	endCh := make(chan struct{})

	timerCh := time.NewTimer(0).C

	go s.electionLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Infow("stopping stolon sentinel")
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

	cd, prevCDPair, err := e.GetClusterData(pctx)
	if err != nil {
		log.Errorw("error retrieving cluster data", zap.Error(err))
		return
	}
	if cd != nil {
		if cd.FormatVersion != cluster.CurrentCDFormatVersion {
			log.Errorw("unsupported clusterdata format version", "version", cd.FormatVersion)
			return
		}
		if err = cd.Cluster.Spec.Validate(); err != nil {
			log.Errorw("clusterdata validation failed", zap.Error(err))
			return
		}
		if cd.Cluster != nil {
			s.sleepInterval = cd.Cluster.DefSpec().SleepInterval.Duration
			s.requestTimeout = cd.Cluster.DefSpec().RequestTimeout.Duration
		}
	}

	log.Debugf("cd dump: %s", spew.Sdump(cd))

	if cd == nil {
		// Cluster first initialization
		if s.initialClusterSpec == nil {
			log.Infow("no cluster data available, waiting for it to appear")
			return
		}
		c := cluster.NewCluster(s.UIDFn(), s.initialClusterSpec)
		log.Infow("writing initial cluster data")
		newcd := cluster.NewClusterData(c)
		log.Debugf("newcd dump: %s", spew.Sdump(newcd))
		if _, err = e.AtomicPutClusterData(pctx, newcd, nil); err != nil {
			log.Errorw("error saving cluster data", zap.Error(err))
		}
		return
	}

	if err = s.setSentinelInfo(pctx, 2*s.sleepInterval); err != nil {
		log.Errorw("cannot update sentinel info", zap.Error(err))
		return
	}

	keepersInfo, err := s.e.GetKeepersInfo(pctx)
	if err != nil {
		log.Errorw("cannot get keepers info", zap.Error(err))
		return
	}
	log.Debugf("keepersInfo dump: %s", spew.Sdump(keepersInfo))

	proxiesInfo, err := s.e.GetProxiesInfo(pctx)
	if err != nil {
		log.Errorw("failed to get proxies info", zap.Error(err))
		return
	}

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
		s.dbNotIncreasingXLogPos = make(map[string]int64)
		s.keeperInfoHistories = make(KeeperInfoHistories)
		s.dbConvergenceInfos = make(map[string]*DBConvergenceInfo)
		s.proxyInfoHistories = make(ProxyInfoHistories)

		// Update db convergence timers since its the first run
		s.updateDBConvergenceInfos(cd)
	}

	newcd, newKeeperInfoHistories := s.updateKeepersStatus(cd, keepersInfo, firstRun)
	log.Debugf("newcd dump after updateKeepersStatus: %s", spew.Sdump(newcd))

	activeProxiesInfos := s.activeProxiesInfos(proxiesInfo)

	newcd, err = s.updateCluster(newcd, activeProxiesInfos)
	if err != nil {
		log.Errorw("failed to update cluster data", zap.Error(err))
		return
	}
	log.Debugf("newcd dump after updateCluster: %s", spew.Sdump(newcd))

	if newcd != nil {
		s.updateChangeTimes(cd, newcd)
		if _, err := e.AtomicPutClusterData(pctx, newcd, prevCDPair); err != nil {
			log.Errorw("error saving clusterdata", zap.Error(err))
		}
	}

	// Save the new keeperInfoHistories only on successful cluster data
	// update or in the next run we'll think that the saved keeperInfo was
	// already applied.
	s.keeperInfoHistories = newKeeperInfoHistories

	// Update db convergence timers using the new cluster data
	s.updateDBConvergenceInfos(newcd)

	// We only update this metric when we've completed all actions in this method
	// successfully. That enables us to alert on when Sentinels are failing to
	// correctly sync.
	lastCheckSuccessSeconds.SetToCurrentTime()
}

func sigHandler(sigs chan os.Signal, cancel context.CancelFunc) {
	s := <-sigs
	log.Debugw("got signal", "signal", s)
	cancel()
}

func Execute() {
	if err := flagutil.SetFlagsFromEnv(CmdSentinel.PersistentFlags(), "STSENTINEL"); err != nil {
		log.Fatal(err)
	}

	if err := CmdSentinel.Execute(); err != nil {
		log.Fatal(err)
	}
}

func sentinel(c *cobra.Command, args []string) {
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

	if err := cmd.CheckCommonConfig(&cfg.CommonConfig); err != nil {
		log.Fatalf(err.Error())
	}

	cmd.SetMetrics(&cfg.CommonConfig, "sentinel")

	uid := common.UID()
	log.Infow("sentinel uid", "uid", uid)

	ctx, cancel := context.WithCancel(context.Background())
	end := make(chan bool)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go sigHandler(sigs, cancel)

	if cfg.MetricsListenAddress != "" {
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			err := http.ListenAndServe(cfg.MetricsListenAddress, nil)
			if err != nil {
				log.Errorw("metrics http server error", zap.Error(err))
				cancel()
			}
		}()
	}

	s, err := NewSentinel(uid, &cfg, end)
	if err != nil {
		log.Fatalf("cannot create sentinel: %v", err)
	}
	go s.Start(ctx)

	// Ensure we collect Sentinel metrics prior to providing Prometheus with an update
	mustRegisterSentinelCollector(s)

	<-end
}
