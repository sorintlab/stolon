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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
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
	"github.com/sorintlab/stolon/pkg/kubernetes"
	"github.com/sorintlab/stolon/pkg/store"

	"github.com/davecgh/go-spew/spew"
	"github.com/docker/leadership"
	"github.com/jmoiron/jsonq"
	"github.com/spf13/cobra"
	"github.com/uber-go/zap"
	"golang.org/x/net/context"
)

var cmdSentinel = &cobra.Command{
	Use: "stolon-sentinel",
	Run: sentinel,
}

const (
	storeDiscovery      = "store"
	kubernetesDiscovery = "kubernetes"
)

type config struct {
	storeBackend            string
	storeEndpoints          string
	clusterName             string
	keeperPort              string
	keeperKubeLabelSelector string
	initialClusterSpecFile  string
	kubernetesNamespace     string
	discoveryType           string
	debug                   bool
}

var cfg config

func init() {
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeBackend, "store-backend", "", "store backend type (etcd or consul)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (defaults: 127.0.0.1:2379 for etcd, 127.0.0.1:8500 for consul)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdSentinel.PersistentFlags().StringVar(&cfg.keeperKubeLabelSelector, "keeper-kube-label-selector", "", "label selector for discoverying stolon-keeper(s) under kubernetes")
	cmdSentinel.PersistentFlags().StringVar(&cfg.keeperPort, "keeper-port", "5431", "stolon-keeper(s) listening port (used by kubernetes discovery)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.initialClusterSpecFile, "initial-cluster-spec", "", "a file providing the initial cluster specification, used only at cluster initialization, ignored if cluster is already initialized")
	cmdSentinel.PersistentFlags().StringVar(&cfg.kubernetesNamespace, "kubernetes-namespace", "default", "the kubernetes namespace stolon is deployed under")
	cmdSentinel.PersistentFlags().StringVar(&cfg.discoveryType, "discovery-type", "", "discovery type (store or kubernetes). Default: detected")
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

func getKeeperInfo(ctx context.Context, kdi *cluster.KeeperDiscoveryInfo) (*cluster.KeeperInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%s/info", kdi.ListenAddress, kdi.Port), nil)
	if err != nil {
		return nil, err
	}
	var data cluster.KeeperInfo
	err = httpDo(ctx, req, nil, func(resp *http.Response, err error) error {
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("http error code: %d, error: %s", resp.StatusCode, resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func GetPGState(ctx context.Context, keeperInfo *cluster.KeeperInfo) (*cluster.PostgresState, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%s/pgstate", keeperInfo.ListenAddress, keeperInfo.Port), nil)
	if err != nil {
		return nil, err
	}
	var pgState cluster.PostgresState
	err = httpDo(ctx, req, nil, func(resp *http.Response, err error) error {
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("http error code: %d, error: %s", resp.StatusCode, resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&pgState); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &pgState, nil
}

func httpDo(ctx context.Context, req *http.Request, tlsConfig *tls.Config, f func(*http.Response, error) error) error {
	// Run the HTTP request in a goroutine and pass the response to f.
	tr := &http.Transport{DisableKeepAlives: true, TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: tr}
	c := make(chan error, 1)
	go func() { c <- f(client.Do(req)) }()
	select {
	case <-ctx.Done():
		tr.CancelRequest(req)
		<-c // Wait for f to return.
		return ctx.Err()
	case err := <-c:
		return err
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

func (s *Sentinel) discover(ctx context.Context) (cluster.KeepersDiscoveryInfo, error) {
	switch s.cfg.discoveryType {
	case storeDiscovery:
		log.Debug("using store discovery")
		return s.discoverStore(ctx)
	case kubernetesDiscovery:
		log.Debug("using kubernetes discovery")
		ksdi := cluster.KeepersDiscoveryInfo{}
		podsIPs, err := s.getKubernetesPodsIPs(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get running pods ips: %v", err)
		}
		for _, podIP := range podsIPs {
			ksdi = append(ksdi, &cluster.KeeperDiscoveryInfo{ListenAddress: podIP, Port: s.cfg.keeperPort})
		}
		return ksdi, nil
	default:
		return nil, fmt.Errorf("unknown discovery type")
	}
}

func (s *Sentinel) discoverStore(ctx context.Context) (cluster.KeepersDiscoveryInfo, error) {
	return s.e.GetKeepersDiscoveryInfo()
}

func (s *Sentinel) getKubernetesPodsIPs(ctx context.Context) ([]string, error) {
	podsIPs := []string{}

	token, err := ioutil.ReadFile("/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve kube api token: %v", err)
	}
	ca, err := ioutil.ReadFile("/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve kube ca certificate: %v", err)
	}
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	u, err := url.Parse(fmt.Sprintf("https://%s:%s/api/v1/namespaces/%s/pods", host, port, cfg.kubernetesNamespace))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("labelSelector", s.cfg.keeperKubeLabelSelector)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))

	roots := x509.NewCertPool()
	if ok := roots.AppendCertsFromPEM([]byte(ca)); !ok {
		return nil, fmt.Errorf("failed to parse kube ca certificate")
	}
	tlsConfig := &tls.Config{RootCAs: roots}
	err = httpDo(ctx, req, tlsConfig, func(resp *http.Response, err error) error {
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return nil
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("http error code: %d, error: %s", resp.StatusCode, resp.Status)
		}
		// Not using kubernetes apis packages since they import tons of other packages
		var data map[string]interface{}
		if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return err
		}

		jq := jsonq.NewQuery(data)

		items, err := jq.ArrayOfObjects("items")
		if err != nil {
			return nil
		}
		for _, item := range items {
			jq := jsonq.NewQuery(item)
			phase, err := jq.String("status", "phase")
			if err != nil {
				log.Error("cannot get pod phase", zap.Error(err))
				return nil
			}
			log.Debug("pod phase", zap.String("phase", phase))
			if phase != "Running" {
				continue
			}
			podIP, err := jq.String("status", "podIP")
			if err != nil {
				log.Error("cannot get pod ip", zap.Error(err))
				return nil
			}
			log.Debug("pod ip", zap.String("ip", podIP))
			podsIPs = append(podsIPs, podIP)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return podsIPs, nil
}

func getKeepersInfo(ctx context.Context, ksdi cluster.KeepersDiscoveryInfo) (cluster.KeepersInfo, error) {
	keepersInfo := make(cluster.KeepersInfo)
	type Response struct {
		idx int
		ki  *cluster.KeeperInfo
		err error
	}
	ch := make(chan Response)
	for idx, kdi := range ksdi {
		go func(idx int, kdi *cluster.KeeperDiscoveryInfo) {
			ki, err := getKeeperInfo(ctx, kdi)
			ch <- Response{idx, ki, err}
		}(idx, kdi)
	}
	count := 0
	for {
		if count == len(ksdi) {
			break
		}
		select {
		case res := <-ch:
			count++
			if res.err != nil {
				log.Error("error getting keeper info", zap.String("address", ksdi[res.idx].ListenAddress), zap.String("port", ksdi[res.idx].Port), zap.Error(res.err))
				break
			}
			keepersInfo[res.ki.UID] = res.ki
		}
	}
	return keepersInfo, nil

}

func getKeepersPGState(ctx context.Context, ki cluster.KeepersInfo) map[string]*cluster.PostgresState {
	keepersPGState := map[string]*cluster.PostgresState{}
	type Response struct {
		id      string
		pgState *cluster.PostgresState
		err     error
	}
	ch := make(chan Response)
	for id, k := range ki {
		go func(id string, k *cluster.KeeperInfo) {
			pgState, err := GetPGState(ctx, k)
			ch <- Response{id, pgState, err}
		}(id, k)
	}
	count := 0
	for {
		if count == len(ki) {
			break
		}
		select {
		case res := <-ch:
			count++
			if res.err != nil {
				log.Error("error getting keeper pg state for keeper", zap.String("keeper", res.id), zap.Error(res.err))
				break
			}
			keepersPGState[res.id] = res.pgState
		}
	}
	return keepersPGState
}

func (s *Sentinel) updateKeepersStatus(cd *cluster.ClusterData, keepersInfo cluster.KeepersInfo) *cluster.ClusterData {
	// Create a copy of cd
	cd = cd.DeepCopy()

	// Create a new keeper from keepersInfo
	for keeperUID, ki := range keepersInfo {
		if _, ok := cd.Keepers[keeperUID]; !ok {
			k := cluster.NewKeeperFromKeeperInfo(ki)
			cd.Keepers[k.UID] = k
		}
	}

	// Update keeper status with keepersInfo
	for keeperUID, ki := range keepersInfo {
		k := cd.Keepers[keeperUID]
		k.Status.ListenAddress = ki.ListenAddress
		k.Status.Port = ki.Port
	}

	// Mark not found keepersInfo as in error
	for keeperUID, k := range cd.Keepers {
		if _, ok := keepersInfo[keeperUID]; !ok {
			k.SetError()
		} else {
			k.CleanError()
		}
	}

	// Update Healthy state
	for _, k := range cd.Keepers {
		k.Status.Healthy = s.isKeeperHealthy(cd, k)
	}

	return cd
}

func (s *Sentinel) updateDBsStatus(cd *cluster.ClusterData, dbStates map[string]*cluster.PostgresState) *cluster.ClusterData {
	// Create newKeepersState as a copy of the current keepersState
	cd = cd.DeepCopy()

	// Update PGstate
	for _, db := range cd.DBs {
		// Mark not found DBs in DBstates in error
		dbs, ok := dbStates[db.Spec.KeeperUID]
		if !ok {
			log.Error("no db state available", zap.String("db", db.UID))
			db.SetError()
			continue
		}
		if dbs.UID != db.UID {
			log.Warn("received db state for unexpected db uid", zap.String("receivedDB", dbs.UID), zap.String("db", db.UID))
			db.SetError()
			continue
		}
		log.Debug("received db state", zap.String("db", db.UID))
		db.Status.ListenAddress = dbs.ListenAddress
		db.Status.Port = dbs.Port
		db.Status.CurrentGeneration = dbs.Generation
		db.Status.PGParameters = cluster.PGParameters(dbs.PGParameters)
		if dbs.Healthy {
			db.CleanError()
			db.Status.SystemID = dbs.SystemID
			db.Status.TimelineID = dbs.TimelineID
			db.Status.XLogPos = dbs.XLogPos
			db.Status.TimelinesHistory = dbs.TimelinesHistory
		}
	}

	// Update Healthy state
	for _, db := range cd.DBs {
		db.Status.Healthy = s.isDBHealthy(cd, db)
	}

	return cd
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
				// TODO(sgotti) set a timeout (the max time for an initdb operation)
				switch s.dbConvergenceState(cd, db, cd.Cluster.Spec.InitTimeout.Duration) {
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
				// TODO(sgotti) set a timeout (the max time for a noop operation, just a start/restart)
				if db.Status.Healthy && s.dbConvergenceState(cd, db, 0) == Converged {
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
				// TODO(sgotti) set a timeout (the max time for an initdb operation)
				switch s.dbConvergenceState(cd, db, cd.Cluster.Spec.InitTimeout.Duration) {
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
		// Add missing DBs
		for _, k := range cd.Keepers {
			if db := cd.FindDB(k); db == nil {
				db := &cluster.DB{
					UID:        s.UIDFn(),
					Generation: cluster.InitialGeneration,
					ChangeTime: time.Now(),
					Spec: &cluster.DBSpec{
						KeeperUID: k.UID,
						InitMode:  cluster.DBInitModeNone,
						Role:      common.RoleUndefined,
						Followers: []string{},
					},
				}
				newcd.DBs[db.UID] = db
			}
		}

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
		if s.dbConvergenceState(cd, curMasterDB, newcd.Cluster.Spec.ConvergenceTimeout.Duration) == ConvergenceFailed {
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
			if masterDB.Status.Healthy && s.dbConvergenceState(newcd, masterDB, newcd.Cluster.Spec.ConvergenceTimeout.Duration) == Converged {
				// Tell proxy that there's a new active master
				newcd.Proxy.Spec.MasterDBUID = wantedMasterDBUID
				newcd.Proxy.ChangeTime = time.Now()

				// TODO(sgotti) do this only for the defined number of MaxStandbysPerSender (needs also to detect unhealthy standbys and switch to healthy one)
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
							// Sort followers
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
	if keeper.Status.ErrorStartTime.IsZero() {
		return true
	}
	if time.Now().After(keeper.Status.ErrorStartTime.Add(cd.Cluster.Spec.FailInterval.Duration)) {
		return false
	}
	return true
}

func (s *Sentinel) isDBHealthy(cd *cluster.ClusterData, db *cluster.DB) bool {
	if db.Status.ErrorStartTime.IsZero() {
		return true
	}
	if time.Now().After(db.Status.ErrorStartTime.Add(cd.Cluster.Spec.FailInterval.Duration)) {
		return false
	}
	return true
}

func (s *Sentinel) dbConvergenceState(cd *cluster.ClusterData, db *cluster.DB, timeout time.Duration) ConvergenceState {
	if db.Status.CurrentGeneration == db.Generation {
		return Converged
	}
	if timeout != 0 {
		if time.Now().After(db.ChangeTime.Add(timeout)) {
			return ConvergenceFailed
		}
	}
	return Converging
}

type Sentinel struct {
	id  string
	cfg *config
	e   *store.StoreManager

	candidate *leadership.Candidate
	stop      chan bool
	end       chan bool

	updateMutex sync.Mutex
	leader      bool
	leaderMutex sync.Mutex

	initialClusterSpec *cluster.ClusterSpec

	sleepInterval  time.Duration
	requestTimeout time.Duration

	// Make UIDFn settable to ease testing with reproducible UIDs
	UIDFn func() string
	// Make RandFn settable to ease testing with reproducible "random" numbers
	RandFn func(int) int
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
	kvstore, err := store.NewStore(store.Backend(cfg.storeBackend), cfg.storeEndpoints)
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

func (s *Sentinel) isLeader() bool {
	s.leaderMutex.Lock()
	defer s.leaderMutex.Unlock()
	return s.leader
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
	keepersDiscoveryInfo, err := s.discover(ctx)
	cancel()
	if err != nil {
		log.Error("err", zap.Error(err))
		return
	}
	log.Debug("keepersDiscoveryInfo dump", zap.String("keepersDiscoveryInfo", spew.Sdump(keepersDiscoveryInfo)))

	ctx, cancel = context.WithTimeout(pctx, s.requestTimeout)
	keepersInfo, err := getKeepersInfo(ctx, keepersDiscoveryInfo)
	cancel()
	if err != nil {
		log.Error("err", zap.Error(err))
		return
	}
	log.Debug("keepersInfo dump", zap.String("keepersInfo", spew.Sdump(keepersInfo)))

	ctx, cancel = context.WithTimeout(pctx, s.requestTimeout)
	keepersPGState := getKeepersPGState(ctx, keepersInfo)
	cancel()
	log.Debug("keepersPGState dump", zap.String("keepersPGState", spew.Sdump(keepersPGState)))

	if !s.isLeader() {
		return
	}

	newcd := s.updateKeepersStatus(cd, keepersInfo)

	newcd = s.updateDBsStatus(newcd, keepersPGState)

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
	if cfg.discoveryType == "" {
		if kubernetes.OnKubernetes() {
			cfg.discoveryType = kubernetesDiscovery
		} else {
			cfg.discoveryType = storeDiscovery
		}
	}
	if cfg.discoveryType != storeDiscovery && cfg.discoveryType != kubernetesDiscovery {
		fmt.Printf("unknown discovery type: %s\n", cfg.discoveryType)
		os.Exit(1)
	}
	if cfg.discoveryType == kubernetesDiscovery {
		if cfg.keeperKubeLabelSelector == "" {
			fmt.Println("keeper-kube-label-selector must be define under kubernetes")
			os.Exit(1)
		}
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
