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
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/gravitational/stolon/common"
	"github.com/gravitational/stolon/pkg/cluster"
	"github.com/gravitational/stolon/pkg/flagutil"
	"github.com/gravitational/stolon/pkg/kubernetes"
	"github.com/gravitational/stolon/pkg/store"

	"github.com/coreos/pkg/capnslog"
	"github.com/davecgh/go-spew/spew"
	"github.com/docker/swarm/leadership"
	"github.com/jmoiron/jsonq"
	"github.com/satori/go.uuid"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

var log = capnslog.NewPackageLogger("github.com/gravitational/stolon/cmd", "sentinel")

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
	storeCertFile           string
	storeKeyFile            string
	storeCACertFile         string
	clusterName             string
	listenAddress           string
	port                    string
	keeperPort              string
	keeperKubeLabelSelector string
	initialClusterConfig    string
	kubernetesNamespace     string
	discoveryType           string
	debug                   bool
}

var cfg config

func init() {
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeBackend, "store-backend", "", "store backend type (etcd or consul)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeEndpoints, "store-endpoints", "", "a comma-delimited list of store endpoints (defaults: 127.0.0.1:2379 for etcd, 127.0.0.1:8500 for consul)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeCertFile, "store-cert", "", "path to the client server TLS cert file")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeKeyFile, "store-key", "", "path to the client server TLS key file")
	cmdSentinel.PersistentFlags().StringVar(&cfg.storeCACertFile, "store-cacert", "", "path to the client server TLS trusted CA key file")
	cmdSentinel.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdSentinel.PersistentFlags().StringVar(&cfg.listenAddress, "listen-address", "localhost", "sentinel listening address")
	cmdSentinel.PersistentFlags().StringVar(&cfg.port, "port", "6431", "sentinel listening port")
	cmdSentinel.PersistentFlags().StringVar(&cfg.keeperKubeLabelSelector, "keeper-kube-label-selector", "", "label selector for discoverying stolon-keeper(s) under kubernetes")
	cmdSentinel.PersistentFlags().StringVar(&cfg.keeperPort, "keeper-port", "5431", "stolon-keeper(s) listening port (used by kubernetes discovery)")
	cmdSentinel.PersistentFlags().StringVar(&cfg.initialClusterConfig, "initial-cluster-config", "", "a file providing the initial cluster config, used only at cluster initialization, ignored if cluster is already initialized")
	cmdSentinel.PersistentFlags().StringVar(&cfg.kubernetesNamespace, "kubernetes-namespace", "default", "the kubernetes namespace stolon is deployed under")
	cmdSentinel.PersistentFlags().StringVar(&cfg.discoveryType, "discovery-type", "", "discovery type (store or kubernetes). Default: detected")
	cmdSentinel.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
}

func init() {
	capnslog.SetFormatter(capnslog.NewPrettyFormatter(os.Stderr, true))
}

func (s *Sentinel) electionLoop() {
	for {
		log.Infof("Trying to acquire sentinels leadership")
		electedCh, errCh, err := s.candidate.RunForElection()
		if err != nil {
			return
		}
		for {
			select {
			case elected := <-electedCh:
				s.leaderMutex.Lock()
				if elected {
					log.Infof("sentinel leadership acquired")
					s.leader = true
				} else {
					if s.leader {
						log.Infof("sentinel leadership lost")
					}
					s.leader = false
				}
				s.leaderMutex.Unlock()

			case err := <-errCh:
				if err != nil {
					log.Errorf("election loop error: %v", err)
				}
				goto end
			case <-s.stop:
				log.Debugf("stopping election Loop")
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
		ID:            s.id,
		ListenAddress: s.listenAddress,
		Port:          s.port,
	}
	log.Debugf(spew.Sprintf("sentinelInfo: %#v", sentinelInfo))

	if err := s.e.SetSentinelInfo(sentinelInfo, ttl); err != nil {
		return err
	}
	return nil
}

func (s *Sentinel) GetBestStandby(cv *cluster.ClusterView, keepersState cluster.KeepersState, master string) (string, error) {
	var bestID string
	masterState := keepersState[master]
	for id, k := range keepersState {
		log.Debugf(spew.Sprintf("id: %s, k: %#v", id, k))
		if id == master {
			log.Debugf("ignoring node %q since it's the current master", id)
			continue
		}
		if !k.Healthy {
			log.Debugf("ignoring node %q since it's not healthy", id)
			continue
		}
		if k.ClusterViewVersion != cv.Version {
			log.Debugf("ignoring node since its clusterView version (%d) is different that the actual one (%d)", k.ClusterViewVersion, cv.Version)
			continue
		}
		if k.PGState == nil {
			log.Debugf("ignoring node since its pg state is unknown")
			continue
		}
		if masterState.PGState.TimelineID != k.PGState.TimelineID {
			log.Debugf("ignoring node since its pg timeline (%s) is different than master timeline (%d)", keepersState[id].PGState.TimelineID, masterState.PGState.TimelineID)
			continue
		}
		if bestID == "" {
			bestID = id
			continue
		}
		if k.PGState.XLogPos > keepersState[bestID].PGState.XLogPos {
			bestID = id
		}
	}
	if bestID == "" {
		return "", fmt.Errorf("no standbys available")
	}
	return bestID, nil
}

func (s *Sentinel) discover(ctx context.Context) (cluster.KeepersDiscoveryInfo, error) {
	switch s.cfg.discoveryType {
	case storeDiscovery:
		log.Debugf("using store discovery")
		return s.discoverStore(ctx)
	case kubernetesDiscovery:
		log.Debugf("using kubernetes discovery")
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
				log.Errorf("cannot get pod phase: %v", err)
				return nil
			}
			log.Debugf("pod phase: %s", phase)
			if phase != "Running" {
				continue
			}
			podIP, err := jq.String("status", "podIP")
			if err != nil {
				log.Errorf("cannot get pod IP: %v", err)
				return nil
			}
			log.Debugf("pod IP: %s", podIP)
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
				log.Errorf("error getting keeper info for %s:%s, err: %v", ksdi[res.idx].ListenAddress, ksdi[res.idx].Port, res.err)
				break
			}
			keepersInfo[res.ki.ID] = res.ki
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
				log.Errorf("error getting keeper pg state for keeper: %s, err: %v", res.id, res.err)
				break
			}
			keepersPGState[res.id] = res.pgState
		}
	}
	return keepersPGState
}

func (s *Sentinel) updateKeepersState(keepersState cluster.KeepersState, keepersInfo cluster.KeepersInfo, keepersPGState map[string]*cluster.PostgresState) cluster.KeepersState {
	// Create newKeepersState as a copy of the current keepersState
	newKeepersState := keepersState.Copy()

	// Add new keepersInfo to newKeepersState
	for id, ki := range keepersInfo {
		if _, ok := newKeepersState[id]; !ok {
			if err := newKeepersState.NewFromKeeperInfo(ki); err != nil {
				// This shouldn't happen
				panic(err)
			}
		}
	}

	// Update keeperState with keepersInfo
	for id, ki := range keepersInfo {
		changed, err := newKeepersState[id].ChangedFromKeeperInfo(ki)
		if err != nil {
			// This shouldn't happen
			panic(err)
		}
		if changed {
			newKeepersState[id].UpdateFromKeeperInfo(ki)
		}
	}

	// Mark not found keepersInfo as in error
	for id, _ := range newKeepersState {
		if _, ok := keepersInfo[id]; !ok {
			newKeepersState[id].SetError()
		} else {
			newKeepersState[id].CleanError()
		}
	}

	// Update PGstate
	for id, k := range newKeepersState {
		if kpg, ok := keepersPGState[id]; !ok {
			newKeepersState[id].SetError()
		} else {
			newKeepersState[id].CleanError()
			k.PGState = kpg
		}
	}

	// Update Healthy state
	for _, k := range newKeepersState {
		k.Healthy = s.isKeeperHealthy(k)
	}

	return newKeepersState
}

func (s *Sentinel) updateClusterView(cv *cluster.ClusterView, keepersState cluster.KeepersState) (*cluster.ClusterView, error) {
	var wantedMasterID string
	if cv.Master == "" {
		if cv.Version != 1 {
			return nil, fmt.Errorf("cluster view at version %d without a defined master. This shouldn't happen!", cv.Version)
		}

		log.Debugf("trying to find initial master")
		// Check for an initial master
		if len(keepersState) < 1 {
			return nil, fmt.Errorf("cannot choose initial master, no keepers registered")
		}
		if len(keepersState) > 1 && !s.clusterConfig.InitWithMultipleKeepers {
			return nil, fmt.Errorf("cannot choose initial master, more than 1 keeper registered")
		}
		for id, k := range keepersState {
			if k.PGState == nil {
				return nil, fmt.Errorf("cannot init cluster using keeper %q since its pg state is unknown", id)
			}
			if !k.PGState.Initialized {
				return nil, fmt.Errorf("cannot init cluster using keeper %q since pg instance is not initializied", id)
			}
			log.Infof("initializing cluster with master: %q", id)
			wantedMasterID = id
			break
		}
	} else {
		masterID := cv.Master
		wantedMasterID = masterID

		masterOK := true
		master, ok := keepersState[masterID]
		if !ok {
			return nil, fmt.Errorf("keeper state for master %q not available. This shouldn't happen!", masterID)
		}
		log.Debugf(spew.Sprintf("masterState: %#v", master))

		if !master.Healthy {
			log.Infof("master is failed")
			masterOK = false
		}

		// Check that the wanted master is in master state (i.e. check that promotion from standby to master happened)
		if !s.isKeeperConverged(master, cv) {
			log.Infof("keeper %s not yet master", masterID)
			masterOK = false
		}

		if !masterOK {
			log.Infof("trying to find a standby to replace failed master")
			bestStandby, err := s.GetBestStandby(cv, keepersState, masterID)
			if err != nil {
				log.Errorf("error trying to find the best standby: %v", err)
			} else {
				if bestStandby != masterID {
					log.Infof("electing new master: %q", bestStandby)
					wantedMasterID = bestStandby
				} else {
					log.Infof("cannot find a good standby to replace failed master")
				}
			}
		}
	}

	newCV := cv.Copy()
	newKeepersRole := newCV.KeepersRole

	// Add new keepersRole from keepersState
	for id, _ := range keepersState {
		if _, ok := newKeepersRole[id]; !ok {
			if err := newKeepersRole.Add(id, ""); err != nil {
				// This shouldn't happen
				panic(err)
			}
		}
	}

	// Setup master role
	if cv.Master != wantedMasterID {
		newCV.Master = wantedMasterID
		newKeepersRole[wantedMasterID].Follow = ""
	}

	// Setup standbys
	if cv.Master == wantedMasterID {
		// wanted master is the previous one
		masterState := keepersState[wantedMasterID]
		// Set standbys to follow master only if it's healthy and converged to the current cv
		if masterState.Healthy && s.isKeeperConverged(masterState, cv) {
			for id, _ := range newKeepersRole {
				if id == wantedMasterID {
					continue
				}
				newKeepersRole[id].Follow = wantedMasterID
			}
		}
	}

	s.updateProxyConf(cv, newCV, keepersState)

	if !newCV.Equals(cv) {
		newCV.Version = cv.Version + 1
		newCV.ChangeTime = time.Now()
	}
	return newCV, nil
}

func (s *Sentinel) updateProxyConf(prevCV *cluster.ClusterView, cv *cluster.ClusterView, keepersState cluster.KeepersState) {
	masterID := cv.Master
	if prevCV.Master != masterID {
		log.Infof("deleting proxyconf")
		// Tell proxy to close connection to old master
		cv.ProxyConf = nil
		return
	}

	master, _ := keepersState[masterID]
	if s.isKeeperConverged(master, prevCV) {
		pc := &cluster.ProxyConf{
			Host: master.PGListenAddress,
			Port: master.PGPort,
		}
		prevPC := prevCV.ProxyConf
		update := true
		if prevPC != nil {
			if prevPC.Host == pc.Host && prevPC.Port == pc.Port {
				update = false
			}
		}
		if update {
			log.Infof("updating proxyconf to %s:%s", pc.Host, pc.Port)
			cv.ProxyConf = pc
		}
	}
	return
}

func (s *Sentinel) isKeeperHealthy(keeperState *cluster.KeeperState) bool {
	if keeperState.ErrorStartTime.IsZero() {
		return true
	}
	if time.Now().After(keeperState.ErrorStartTime.Add(s.clusterConfig.KeeperFailInterval)) {
		return false
	}
	return true
}

func (s *Sentinel) isKeeperConverged(keeperState *cluster.KeeperState, cv *cluster.ClusterView) bool {
	if keeperState.ClusterViewVersion != cv.Version {
		if time.Now().After(cv.ChangeTime.Add(s.clusterConfig.KeeperFailInterval)) {
			return false
		}
	}
	return true
}

type Sentinel struct {
	id  string
	cfg *config
	e   *store.StoreManager

	candidate *leadership.Candidate
	stop      chan bool
	end       chan bool

	listenAddress string
	port          string

	clusterConfig           *cluster.Config
	initialClusterNilConfig *cluster.NilConfig

	updateMutex sync.Mutex
	leader      bool
	leaderMutex sync.Mutex
}

func NewSentinel(id string, cfg *config, stop chan bool, end chan bool) (*Sentinel, error) {
	var initialClusterNilConfig *cluster.NilConfig
	if cfg.initialClusterConfig != "" {
		configData, err := ioutil.ReadFile(cfg.initialClusterConfig)
		if err != nil {
			return nil, fmt.Errorf("cannot read provided initial cluster config file: %v", err)
		}
		if err := json.Unmarshal(configData, &initialClusterNilConfig); err != nil {
			return nil, fmt.Errorf("cannot parse provided initial cluster config: %v", err)
		}
	}

	storePath := filepath.Join(common.StoreBasePath, cfg.clusterName)
	kvstore, err := store.NewStore(
		store.Backend(cfg.storeBackend),
		cfg.storeEndpoints,
		cfg.storeCertFile,
		cfg.storeKeyFile,
		cfg.storeCACertFile,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}
	e := store.NewStoreManager(kvstore, storePath)

	candidate := leadership.NewCandidate(kvstore, filepath.Join(storePath, common.SentinelLeaderKey), id, 15*time.Second)

	return &Sentinel{
		id:                      id,
		cfg:                     cfg,
		e:                       e,
		listenAddress:           cfg.listenAddress,
		port:                    cfg.port,
		candidate:               candidate,
		leader:                  false,
		initialClusterNilConfig: initialClusterNilConfig,
		stop: stop,
		end:  end}, nil
}

func (s *Sentinel) Start() {
	endCh := make(chan struct{})
	endApiCh := make(chan error)

	router := s.NewRouter()
	go func() {
		endApiCh <- http.ListenAndServe(fmt.Sprintf("%s:%s", s.listenAddress, s.port), router)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	timerCh := time.NewTimer(0).C

	go s.electionLoop()

	for true {
		select {
		case <-s.stop:
			log.Debugf("stopping stolon sentinel")
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
			var sleepInterval time.Duration
			if s.clusterConfig == nil {
				sleepInterval = cluster.DefaultSleepInterval
			} else {
				sleepInterval = s.clusterConfig.SleepInterval
			}
			timerCh = time.NewTimer(sleepInterval).C
		case err := <-endApiCh:
			if err != nil {
				log.Fatal("ListenAndServe: ", err)
			}
			close(s.stop)
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
		log.Errorf("error retrieving cluster data: %v", err)
		return
	}

	var cv *cluster.ClusterView
	var keepersState cluster.KeepersState
	if cd == nil {
		cv = cluster.NewClusterView()
		keepersState = nil
	} else {
		cv = cd.ClusterView
		keepersState = cd.KeepersState
	}
	log.Debugf(spew.Sprintf("keepersState: %#v", keepersState))
	log.Debugf(spew.Sprintf("clusterView: %#v", cv))

	// Update cluster config
	// This shouldn't need a lock
	s.clusterConfig = cv.Config.ToConfig()

	if err = s.setSentinelInfo(2 * s.clusterConfig.SleepInterval); err != nil {
		log.Errorf("cannot update sentinel info: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(pctx, s.clusterConfig.RequestTimeout)
	keepersDiscoveryInfo, err := s.discover(ctx)
	cancel()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("keepersDiscoveryInfo: %#v", keepersDiscoveryInfo))

	ctx, cancel = context.WithTimeout(pctx, s.clusterConfig.RequestTimeout)
	keepersInfo, err := getKeepersInfo(ctx, keepersDiscoveryInfo)
	cancel()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("keepersInfo: %#v", keepersInfo))

	ctx, cancel = context.WithTimeout(pctx, s.clusterConfig.RequestTimeout)
	keepersPGState := getKeepersPGState(ctx, keepersInfo)
	cancel()
	log.Debugf(spew.Sprintf("keepersPGState: %#v", keepersPGState))

	if !s.isLeader() {
		return
	}

	if cv.Version == 0 {
		log.Infof("Initializing cluster")
		// Cluster first initialization
		newcv := cluster.NewClusterView()
		newcv.Version = 1
		if s.initialClusterNilConfig != nil {
			newcv.Config = s.initialClusterNilConfig
		}
		log.Debugf(spew.Sprintf("new clusterView: %#v", newcv))
		if _, err = e.SetClusterData(nil, newcv, nil); err != nil {
			log.Errorf("error saving clusterdata: %v", err)
		}
		return
	}

	newKeepersState := s.updateKeepersState(keepersState, keepersInfo, keepersPGState)
	log.Debugf(spew.Sprintf("newKeepersState: %#v", newKeepersState))

	newcv, err := s.updateClusterView(cv, newKeepersState)
	if err != nil {
		log.Errorf("failed to update clusterView: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("newcv: %#v", newcv))
	if cv.Version < newcv.Version {
		log.Debugf("newcv changed from previous cv")
	}

	if _, err := e.SetClusterData(newKeepersState, newcv, prevCDPair); err != nil {
		log.Errorf("error saving clusterdata: %v", err)
	}
}

func sigHandler(sigs chan os.Signal, stop chan bool) {
	s := <-sigs
	log.Debugf("got signal: %s", s)
	close(stop)
}

func main() {
	flagutil.SetFlagsFromEnv(cmdSentinel.PersistentFlags(), "STSENTINEL")

	cmdSentinel.Execute()
}

func sentinel(cmd *cobra.Command, args []string) {
	capnslog.SetGlobalLogLevel(capnslog.INFO)
	if cfg.debug {
		capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	}
	if cfg.clusterName == "" {
		log.Fatalf("cluster name required")
	}
	if cfg.storeBackend == "" {
		log.Fatalf("store backend type required")
	}
	if cfg.discoveryType == "" {
		if kubernetes.OnKubernetes() {
			cfg.discoveryType = kubernetesDiscovery
		} else {
			cfg.discoveryType = storeDiscovery
		}
	}
	if cfg.discoveryType != storeDiscovery && cfg.discoveryType != kubernetesDiscovery {
		log.Fatalf("unknown discovery type: %s", cfg.discoveryType)
	}
	if cfg.discoveryType == kubernetesDiscovery {
		if cfg.keeperKubeLabelSelector == "" {
			log.Fatalf("keeper-kube-label-selector must be define under kubernetes")
		}
	}

	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])
	log.Infof("id: %s", id)

	stop := make(chan bool, 0)
	end := make(chan bool, 0)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill)
	go sigHandler(sigs, stop)

	s, err := NewSentinel(id, &cfg, stop, end)
	if err != nil {
		log.Fatalf("cannot create sentinel: %v", err)
	}
	go s.Start()

	<-end
}
