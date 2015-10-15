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
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	etcdm "github.com/sorintlab/stolon/pkg/etcd"
	"github.com/sorintlab/stolon/pkg/flagutil"
	"github.com/sorintlab/stolon/pkg/kubernetes"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/fleet/pkg/lease"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/pkg/capnslog"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/jmoiron/jsonq"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/spf13/cobra"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
)

var log = capnslog.NewPackageLogger("github.com/sorintlab/stolon/cmd", "sentinel")

var cmdSentinel = &cobra.Command{
	Use: "stolon-sentinel",
	Run: sentinel,
}

type config struct {
	etcdEndpoints           string
	clusterName             string
	keeperPort              string
	keeperKubeLabelSelector string
	debug                   bool
}

var cfg config

func init() {
	cmdSentinel.PersistentFlags().StringVar(&cfg.etcdEndpoints, "etcd-endpoints", "http://127.0.0.1:4001,http://127.0.0.1:2379", "a comma-delimited list of etcd endpoints")
	cmdSentinel.PersistentFlags().StringVar(&cfg.clusterName, "cluster-name", "", "cluster name")
	cmdSentinel.PersistentFlags().StringVar(&cfg.keeperKubeLabelSelector, "keeper-kube-label-selector", "", "label selector for discoverying stolon-keeper(s) under kubernetes")
	cmdSentinel.PersistentFlags().StringVar(&cfg.keeperPort, "keeper-port", "5431", "stolon-keeper(s) listening port (used by kubernetes discovery)")
	cmdSentinel.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "enable debug logging")
}

func init() {
	capnslog.SetFormatter(capnslog.NewPrettyFormatter(os.Stderr, true))
}

func acquireLeadership(lManager lease.Manager, machID string, ver int, ttl time.Duration) lease.Lease {
	existing, err := lManager.GetLease(common.SentinelLeaseName)
	if err != nil {
		log.Errorf("unable to determine current lessee: %v", err)
		return nil
	}

	var l lease.Lease
	if existing == nil {
		l, err = lManager.AcquireLease(common.SentinelLeaseName, machID, ver, ttl)
		if err != nil {
			log.Errorf("sentinel leadership acquisition failed: %v", err)
			return nil
		} else if l == nil {
			log.Debugf("unable to acquire sentinel leadership")
			return nil
		}
		log.Infof("sentinel leadership acquired")
		return l
	}

	if existing.Version() >= ver {
		log.Debugf("lease already held by Machine(%s) operating at acceptable version %d", existing.MachineID(), existing.Version())
		return existing
	}

	rem := existing.TimeRemaining()
	l, err = lManager.StealLease(common.SentinelLeaseName, machID, ver, ttl+rem, existing.Index())
	if err != nil {
		log.Errorf("sentinel leadership steal failed: %v", err)
		return nil
	} else if l == nil {
		log.Debugf("unable to steal sentinel leadership")
		return nil
	}

	log.Infof("stole sentinel leadership from Machine(%s)", existing.MachineID())

	if rem > 0 {
		log.Infof("waiting %v for previous lease to expire before continuing reconciliation", rem)
		<-time.After(rem)
	}

	return l
}

func renewLeadership(l lease.Lease, ttl time.Duration) lease.Lease {
	err := l.Renew(ttl)

	if err != nil {
		log.Errorf("sentinel leadership lost, renewal failed: %v", err)
		return nil
	}

	log.Debugf("sentinel leadership renewed")
	return l
}

func isLeader(l lease.Lease, machID string) bool {
	if l == nil {
		return false
	}
	if l.MachineID() != machID {
		return false
	}
	return true
}

func getMemberInfo(ctx context.Context, di *cluster.MemberDiscoveryInfo) (*cluster.MemberInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%s/info", di.Host, di.Port), nil)
	if err != nil {
		return nil, err
	}
	var data cluster.MemberInfo
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

func GetPGState(ctx context.Context, memberInfo *cluster.MemberInfo) (*cluster.PostgresState, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%s/pgstate", memberInfo.Host, memberInfo.Port), nil)
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

func (s *Sentinel) GetBestStandby(cv *cluster.ClusterView, membersState cluster.MembersState, master string) (string, error) {
	var bestID string
	masterState := membersState[master]
	for id, m := range membersState {
		log.Debugf(spew.Sprintf("id: %s, m: %#v", id, m))
		if id == master {
			log.Debugf("ignoring node %s as it's the current master", id)
			continue
		}
		if !s.isMemberHealthy(m) {
			log.Debugf("ignoring node %s as it's not healthy", id)
			continue
		}
		if m.ClusterViewVersion != cv.Version {
			log.Debugf("ignoring node as its clusterView version (%d) is different that the actual one (%d)", m.ClusterViewVersion, cv.Version)
			continue
		}
		if m.PGState == nil {
			log.Debugf("ignoring node as its pg state is unknown")
			continue
		}
		if masterState.PGState.TimelineID != m.PGState.TimelineID {
			log.Debugf("ignoring node as its pg timeline (%s) is different than master timeline (%d)", membersState[id].PGState.TimelineID, masterState.PGState.TimelineID)
			continue
		}
		if bestID == "" {
			bestID = id
			continue
		}
		if membersState[id].PGState.XLogPos > m.PGState.XLogPos {
			bestID = id
		}
	}
	if bestID == "" {
		return "", fmt.Errorf("no standbys available")
	}
	return bestID, nil
}

func (s *Sentinel) discover(ctx context.Context) (cluster.MembersDiscoveryInfo, error) {
	if kubernetes.OnKubernetes() {
		msdi := cluster.MembersDiscoveryInfo{}
		log.Debugf("running inside kubernetes")
		podsIPs, err := s.getKubernetesPodsIPs(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get running pods ips: %v", err)
		}
		for _, podIP := range podsIPs {
			msdi = append(msdi, &cluster.MemberDiscoveryInfo{Host: podIP, Port: cfg.keeperPort})
		}
		return msdi, nil
	}

	return s.discoverEtcd(ctx)
}

func (s *Sentinel) discoverEtcd(ctx context.Context) (cluster.MembersDiscoveryInfo, error) {
	return s.e.GetMembersDiscoveryInfo()
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
	u, err := url.Parse(fmt.Sprintf("https://%s:%s/api/v1/namespaces/default/pods", host, port))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("labelSelector", cfg.keeperKubeLabelSelector)
	u.RawQuery = q.Encode()

	log.Debugf("u: %s", u.String())

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
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
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

func getMembersInfo(ctx context.Context, mdi cluster.MembersDiscoveryInfo) (cluster.MembersInfo, error) {
	membersInfo := make(cluster.MembersInfo)
	type Response struct {
		idx int
		mi  *cluster.MemberInfo
		err error
	}
	ch := make(chan Response)
	for idx, m := range mdi {
		go func(idx int, m *cluster.MemberDiscoveryInfo) {
			mi, err := getMemberInfo(ctx, m)
			ch <- Response{idx, mi, err}
		}(idx, m)
	}
	count := 0
	for {
		if count == len(mdi) {
			break
		}
		select {
		case res := <-ch:
			count++
			if res.err != nil {
				log.Errorf("error getting member info for %s:%s, err: %v", mdi[res.idx].Host, mdi[res.idx].Port, res.err)
				break
			}
			membersInfo[res.mi.ID] = res.mi
		}
	}
	return membersInfo, nil

}

func getMembersPGState(ctx context.Context, mi cluster.MembersInfo) map[string]*cluster.PostgresState {
	membersPGState := map[string]*cluster.PostgresState{}
	type Response struct {
		id      string
		pgState *cluster.PostgresState
		err     error
	}
	ch := make(chan Response)
	for id, m := range mi {
		go func(id string, m *cluster.MemberInfo) {
			pgState, err := GetPGState(ctx, m)
			ch <- Response{id, pgState, err}
		}(id, m)
	}
	count := 0
	for {
		if count == len(mi) {
			break
		}
		select {
		case res := <-ch:
			count++
			if res.err != nil {
				log.Errorf("error getting member pg state for member: %s, err: %v", res.id, res.err)
				break
			}
			membersPGState[res.id] = res.pgState
		}
	}
	return membersPGState
}

func (s *Sentinel) updateMembersState(membersState cluster.MembersState, membersInfo cluster.MembersInfo, membersPGState map[string]*cluster.PostgresState) cluster.MembersState {
	// Create newMembersState as a copy of the current membersState
	newMembersState := membersState.Copy()

	// Add new membersInfo to newMembersState
	for id, mi := range membersInfo {
		if _, ok := newMembersState[id]; !ok {
			newMembersState[id] = &cluster.MemberState{
				ErrorStartTime:     time.Time{},
				ID:                 mi.ID,
				ClusterViewVersion: mi.ClusterViewVersion,
				Host:               mi.Host,
				Port:               mi.Port,
				PGListenAddress:    mi.PGListenAddress,
				PGPort:             mi.PGPort,
			}
		}
	}

	// Update memberState with membersInfo
	for id, mi := range membersInfo {
		if mi.Changed(newMembersState[id]) {
			newMembersState[id] = &cluster.MemberState{
				ID:                 mi.ID,
				ClusterViewVersion: mi.ClusterViewVersion,
				Host:               mi.Host,
				Port:               mi.Port,
				PGListenAddress:    mi.PGListenAddress,
				PGPort:             mi.PGPort,
			}
		}
	}

	// Mark not found membersInfo as in error
	for id, _ := range newMembersState {
		if _, ok := membersInfo[id]; !ok {
			newMembersState[id].MarkError()
		} else {
			newMembersState[id].MarkOK()
		}
	}

	// Update PGstate
	for id, m := range newMembersState {
		if mpg, ok := membersPGState[id]; ok {
			m.PGState = mpg
		} else {
			newMembersState[id].MarkError()
		}
	}

	return newMembersState
}

func (s *Sentinel) updateClusterView(cv *cluster.ClusterView, membersState cluster.MembersState) (*cluster.ClusterView, error) {
	var wantedMasterID string
	// Cluster first initialization
	if cv.Version == 0 {
		log.Debugf("finding initial master")
		// Check for an initial master
		if len(membersState) < 1 {
			return nil, fmt.Errorf("cannot init cluster, no members registered")
		}
		if len(membersState) > 1 {
			return nil, fmt.Errorf("cannot init cluster, more than 1 member registered")
		}
		for id, _ := range membersState {
			log.Debugf("masterID: %s", id)
			if id != "" {
				log.Infof("Initializing cluster with master: %s", id)
				wantedMasterID = id
			}
			break
		}
	} else {
		masterID := cv.Master
		log.Debugf("masterID: %s", masterID)

		masterOK := true
		master, ok := membersState[masterID]
		if !ok {
			return nil, fmt.Errorf("member state for master %q not available. This shouldn't happen!", masterID)
		}
		log.Debugf(spew.Sprintf("master: %#v", master))

		if !s.isMemberHealthy(master) {
			log.Infof("master is failed")
			masterOK = false
		}

		// Check that the wanted master is in master state (i.e. check that promotion from standby to master happened)
		if !s.isMemberConverged(master, cv) {
			log.Infof("member %s not yet master", masterID)
			masterOK = false
		}

		wantedMasterID = masterID
		if !masterOK {
			log.Infof("trying to find a standby to replace failed master")
			bestStandby, err := s.GetBestStandby(cv, membersState, masterID)
			if err != nil {
				log.Errorf("error trying to find the best standby: %v", err)
			} else {
				if bestStandby != masterID {
					log.Debugf("electing new master: %s", bestStandby)
					wantedMasterID = bestStandby
				} else {
					log.Infof("cannot find a good standby to replace failed master")
				}
			}
		}
	}

	newCV := cv.Copy()
	newMembersRole := newCV.MembersRole

	// Add new members from membersState
	for id, _ := range membersState {
		if _, ok := newMembersRole[id]; !ok {
			newMembersRole[id] = &cluster.MemberRole{}
		}

	}

	// Setup master role
	if cv.Master != wantedMasterID {
		newCV.Master = wantedMasterID
		newMembersRole[wantedMasterID] = &cluster.MemberRole{Follow: ""}
	}

	// Setup standbys
	if cv.Master == wantedMasterID {
		// wanted master is the previous one
		masterState := membersState[wantedMasterID]
		if s.isMemberHealthy(masterState) && s.isMemberConverged(masterState, cv) {
			for id, _ := range newMembersRole {
				if id == wantedMasterID {
					continue
				}
				newMembersRole[id] = &cluster.MemberRole{Follow: wantedMasterID}
			}
		}
	}

	if !newCV.Equals(cv) {
		newCV.Version = cv.Version + 1
		newCV.ChangeTime = time.Now()
	}
	return newCV, nil
}

func (s *Sentinel) updateProxyView(prevCV *cluster.ClusterView, cv *cluster.ClusterView, membersState cluster.MembersState, prevPVIndex uint64) error {
	if prevCV != nil && cv != nil {
		if prevCV.Master != cv.Master {
			if prevPVIndex != 0 {
				log.Infof("deleting proxy view")
				// Tell proxy to close connection to old master
				_, err := s.e.DeleteProxyView(prevPVIndex)
				return err
			}
		}
	}
	if prevCV != nil {
		masterID := cv.Master
		master, ok := membersState[masterID]
		if !ok {
			return fmt.Errorf("member info for master %q not available. This shouldn't happen!", masterID)
		}
		if s.isMemberConverged(master, prevCV) {
			pv := &cluster.ProxyView{
				Host: master.PGListenAddress,
				Port: master.PGPort,
			}
			log.Infof("Updating proxy view to %s:%s", pv.Host, pv.Port)
			_, err := s.e.SetProxyView(pv, prevPVIndex)
			return err
		}
	}
	return nil
}

func (s *Sentinel) isMemberHealthy(memberState *cluster.MemberState) bool {
	if memberState.ErrorStartTime.IsZero() {
		return true
	}
	if time.Now().After(memberState.ErrorStartTime.Add(s.clusterConfig.MemberFailInterval)) {
		return false
	}
	return true
}

func (s *Sentinel) isMemberConverged(memberState *cluster.MemberState, cv *cluster.ClusterView) bool {
	if memberState.ClusterViewVersion != cv.Version {
		if time.Now().After(cv.ChangeTime.Add(s.clusterConfig.MemberFailInterval)) {
			return false
		}
	}
	return true
}

type Sentinel struct {
	id            string
	e             *etcdm.EtcdManager
	lManager      lease.Manager
	l             lease.Lease
	stop          chan bool
	end           chan bool
	clusterConfig *cluster.Config
}

func NewSentinel(id string, cfg config, stop chan bool, end chan bool) (*Sentinel, error) {
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

	lManager := e.NewLeaseManager()

	return &Sentinel{id: id, e: e, lManager: lManager, clusterConfig: clusterConfig, stop: stop, end: end}, nil
}

func (s *Sentinel) Start() {
	endCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	timerCh := time.NewTimer(0).C

	for true {
		select {
		case <-s.stop:
			log.Debugf("stopping postgres keeper")
			cancel()
			s.end <- true
			return
		case <-timerCh:
			go func() {
				s.clusterSentinelSM(ctx)
				endCh <- struct{}{}
			}()
		case <-endCh:
			timerCh = time.NewTimer(s.clusterConfig.SleepInterval).C
		}
	}
}

func (s *Sentinel) clusterSentinelSM(pctx context.Context) {
	e := s.e

	// Update cluster config
	clusterConfig, _, err := e.GetClusterConfig()
	if err != nil {
		log.Errorf("cannot get cluster config: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("clusterConfig: %+v", clusterConfig))
	// This shouldn't need a lock
	s.clusterConfig = clusterConfig

	// TODO(sgotti) better ways to calculate leaseTTL?
	leaseTTL := clusterConfig.SleepInterval + clusterConfig.RequestTimeout*4

	ctx, cancel := context.WithTimeout(pctx, s.clusterConfig.RequestTimeout)
	membersDiscoveryInfo, err := s.discover(ctx)
	cancel()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("membersDiscoveryInfo: %#v", membersDiscoveryInfo))

	ctx, cancel = context.WithTimeout(pctx, s.clusterConfig.RequestTimeout)
	membersInfo, err := getMembersInfo(ctx, membersDiscoveryInfo)
	cancel()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("membersInfo: %#v", membersInfo))

	ctx, cancel = context.WithTimeout(pctx, s.clusterConfig.RequestTimeout)
	membersPGState := getMembersPGState(ctx, membersInfo)
	cancel()
	log.Debugf(spew.Sprintf("membersPGState: %#v", membersPGState))

	var l lease.Lease
	if isLeader(s.l, s.id) {
		log.Infof("I'm the sentinels leader")
		l = renewLeadership(s.l, leaseTTL)
	} else {
		log.Infof("trying to acquire sentinels leadership")
		l = acquireLeadership(s.lManager, s.id, 1, leaseTTL)
	}

	// log all leadership changes
	if l != nil && s.l == nil && l.MachineID() != s.id {
		log.Infof("sentinel leader is %s", l.MachineID())
	} else if l != nil && s.l != nil && l.MachineID() != l.MachineID() {
		log.Infof("sentinel leadership changed from %s to %s", l.MachineID(), l.MachineID())
	}

	s.l = l

	if !isLeader(s.l, s.id) {
		return
	}

	cd, res, err := e.GetClusterData()
	if err != nil {
		log.Errorf("error retrieving cluster data: %v", err)
		return
	}
	var prevCDIndex uint64
	if res != nil {
		prevCDIndex = res.Node.ModifiedIndex
	}

	var cv *cluster.ClusterView
	var membersState cluster.MembersState
	if cd == nil {
		cv = cluster.NewClusterView()
		membersState = nil
	} else {
		cv = cd.ClusterView
		membersState = cd.MembersState
	}
	log.Debugf(spew.Sprintf("membersState: %#v", membersState))
	log.Debugf(spew.Sprintf("clusterView: %v", cv))

	pv, res, err := e.GetProxyView()
	if err != nil {
		log.Errorf("err: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("proxyview: %v", pv))

	var prevPVIndex uint64
	if res != nil {
		prevPVIndex = res.Node.ModifiedIndex
	}

	newMembersState := s.updateMembersState(membersState, membersInfo, membersPGState)
	log.Debugf(spew.Sprintf("newMembersState: %#v", newMembersState))

	newcv, err := s.updateClusterView(cv, newMembersState)
	if err != nil {
		log.Errorf("failed to update clusterView: %v", err)
		return
	}
	log.Debugf(spew.Sprintf("newcv: %#v", newcv))
	if cv.Version < newcv.Version {
		log.Debugf("newcv changed from previous cv")
		if err := s.updateProxyView(cv, newcv, newMembersState, prevPVIndex); err != nil {
			log.Errorf("error updating proxyView: %v", err)
			return
		}
	}

	_, err = e.SetClusterData(newMembersState, newcv, prevCDIndex)
	if err != nil {
		log.Errorf("error saving clusterdata: %v", err)
	}
}

func sigHandler(sigs chan os.Signal, stop chan bool) {
	s := <-sigs
	log.Debugf("Got signal: %s", s)
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
	if kubernetes.OnKubernetes() {
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

	s, err := NewSentinel(id, cfg, stop, end)
	if err != nil {
		log.Fatalf("cannot create sentinel: %v", err)
	}
	go s.Start()

	<-end
}
