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

package integration

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	pg "github.com/sorintlab/stolon/pkg/postgresql"
	"github.com/sorintlab/stolon/pkg/store"
	"github.com/sorintlab/stolon/pkg/util"

	kvstore "github.com/docker/libkv/store"
	_ "github.com/lib/pq"
	"github.com/satori/go.uuid"
	"github.com/sgotti/gexpect"
)

const (
	sleepInterval = 500 * time.Millisecond

	MinPort = 2048
	MaxPort = 16384
)

var (
	defaultPGParameters = cluster.PGParameters{"log_destination": "stderr", "logging_collector": "false"}
)

var curPort = MinPort
var portMutex = sync.Mutex{}

func pgParametersWithDefaults(p cluster.PGParameters) cluster.PGParameters {
	pd := cluster.PGParameters{}
	for k, v := range defaultPGParameters {
		pd[k] = v
	}
	for k, v := range p {
		pd[k] = v
	}
	return pd
}

type Querier interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

func GetPGParameters(q Querier) (common.Parameters, error) {
	var pgParameters = common.Parameters{}
	rows, err := q.Query("select name, setting, source from pg_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, setting, source string
		if err = rows.Scan(&name, &setting, &source); err != nil {
			return nil, err
		}
		if source == "configuration file" {
			pgParameters[name] = setting
		}
	}
	return pgParameters, nil
}

type Process struct {
	t    *testing.T
	uid  string
	name string
	args []string
	cmd  *gexpect.ExpectSubprocess
	bin  string
}

func (p *Process) start() error {
	if p.cmd != nil {
		panic(fmt.Errorf("%s: cmd not cleanly stopped", p.uid))
	}
	cmd := exec.Command(p.bin, p.args...)
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	p.cmd = &gexpect.ExpectSubprocess{Cmd: cmd, Output: pw}
	if err := p.cmd.Start(); err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			p.t.Logf("[%s %s]: %s", p.name, p.uid, scanner.Text())
		}
	}()

	return nil
}

func (p *Process) Start() error {
	if err := p.start(); err != nil {
		return err
	}
	p.cmd.Continue()
	return nil
}

func (p *Process) StartExpect() error {
	return p.start()
}

func (p *Process) Signal(sig os.Signal) error {
	p.t.Logf("signalling %s %s with %s", p.name, p.uid, sig)
	if p.cmd == nil {
		panic(fmt.Errorf("p: %s, cmd is empty", p.uid))
	}
	return p.cmd.Cmd.Process.Signal(sig)
}

func (p *Process) Kill() {
	p.t.Logf("killing %s %s", p.name, p.uid)
	if p.cmd == nil {
		panic(fmt.Errorf("p: %s, cmd is empty", p.uid))
	}
	p.cmd.Cmd.Process.Signal(os.Kill)
	p.cmd.Wait()
	p.cmd = nil
}

func (p *Process) Stop() {
	p.t.Logf("stopping %s %s", p.name, p.uid)
	if p.cmd == nil {
		panic(fmt.Errorf("p: %s, cmd is empty", p.uid))
	}
	p.cmd.Continue()
	p.cmd.Cmd.Process.Signal(os.Interrupt)
	p.cmd.Wait()
	p.cmd = nil
}

func (p *Process) Wait(timeout time.Duration) error {
	timeoutCh := time.NewTimer(timeout).C
	endCh := make(chan error)
	go func() {
		err := p.cmd.Wait()
		endCh <- err
	}()
	select {
	case <-timeoutCh:
		return fmt.Errorf("timeout waiting on process")
	case <-endCh:
		return nil
	}
}

type TestKeeper struct {
	t *testing.T
	Process
	dataDir         string
	pgListenAddress string
	pgPort          string
	pgSUUsername    string
	pgSUPassword    string
	pgReplUsername  string
	pgReplPassword  string
	db              *sql.DB
}

func NewTestKeeperWithID(t *testing.T, dir, uid, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestKeeper, error) {
	args := []string{}

	dataDir := filepath.Join(dir, fmt.Sprintf("st%s", uid))

	pgListenAddress, pgPort, err := getFreePort(true, false)
	if err != nil {
		return nil, err
	}

	args = append(args, fmt.Sprintf("--uid=%s", uid))
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))
	args = append(args, fmt.Sprintf("--pg-listen-address=%s", pgListenAddress))
	args = append(args, fmt.Sprintf("--pg-port=%s", pgPort))
	args = append(args, fmt.Sprintf("--data-dir=%s", dataDir))
	args = append(args, fmt.Sprintf("--store-backend=%s", storeBackend))
	args = append(args, fmt.Sprintf("--store-endpoints=%s", storeEndpoints))
	args = append(args, fmt.Sprintf("--pg-su-username=%s", pgSUUsername))
	if pgSUPassword != "" {
		args = append(args, fmt.Sprintf("--pg-su-password=%s", pgSUPassword))
	}
	args = append(args, fmt.Sprintf("--pg-repl-username=%s", pgReplUsername))
	args = append(args, fmt.Sprintf("--pg-repl-password=%s", pgReplPassword))
	if os.Getenv("DEBUG") != "" {
		args = append(args, "--debug")
	}
	args = append(args, a...)

	connParams := pg.ConnParams{
		"user":     pgSUUsername,
		"password": pgSUPassword,
		"host":     pgListenAddress,
		"port":     pgPort,
		"dbname":   "postgres",
		"sslmode":  "disable",
	}

	connString := connParams.ConnString()
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	bin := os.Getenv("STKEEPER_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing STKEEPER_BIN env")
	}
	tk := &TestKeeper{
		t: t,
		Process: Process{
			t:    t,
			uid:  uid,
			name: "keeper",
			bin:  bin,
			args: args,
		},
		dataDir:         dataDir,
		pgListenAddress: pgListenAddress,
		pgPort:          pgPort,
		pgSUUsername:    pgSUUsername,
		pgSUPassword:    pgSUPassword,
		pgReplUsername:  pgReplUsername,
		pgReplPassword:  pgReplPassword,
		db:              db,
	}
	return tk, nil
}

func NewTestKeeper(t *testing.T, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestKeeper, error) {
	u := uuid.NewV4()
	uid := fmt.Sprintf("%x", u[:4])

	return NewTestKeeperWithID(t, dir, uid, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, storeBackend, storeEndpoints, a...)
}

func (tk *TestKeeper) PGDataVersion() (int, int, error) {
	fh, err := os.Open(filepath.Join(tk.dataDir, "postgres", "PG_VERSION"))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read PG_VERSION: %v", err)
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	scanner.Split(bufio.ScanLines)

	scanner.Scan()

	version := scanner.Text()
	return pg.ParseVersion(version)
}

func (tk *TestKeeper) GetPrimaryConninfo() (pg.ConnParams, error) {
	regex := regexp.MustCompile(`\s*primary_conninfo\s*=\s*'(.*)'$`)

	fh, err := os.Open(filepath.Join(tk.dataDir, "postgres", "recovery.conf"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		m := regex.FindStringSubmatch(scanner.Text())
		if len(m) == 2 {
			return pg.ParseConnString(m[1])
		}
	}
	return nil, nil
}

func (tk *TestKeeper) Exec(query string, args ...interface{}) (sql.Result, error) {
	res, err := tk.db.Exec(query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tk *TestKeeper) Query(query string, args ...interface{}) (*sql.Rows, error) {
	res, err := tk.db.Query(query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tk *TestKeeper) WaitDBUp(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := tk.Exec("select 1")
		if err == nil {
			return nil
		}
		tk.t.Logf("tk: %v, error: %v", tk.uid, err)
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func (tk *TestKeeper) WaitDBDown(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := tk.Exec("select 1")
		if err != nil {
			return nil
		}
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func (tk *TestKeeper) GetPGProcess() (*os.Process, error) {
	fh, err := os.Open(filepath.Join(tk.dataDir, "postgres/postmaster.pid"))
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	scanner.Split(bufio.ScanLines)
	if !scanner.Scan() {
		return nil, fmt.Errorf("not enough lines in pid file")
	}
	pidStr := scanner.Text()
	pid, err := strconv.Atoi(string(pidStr))
	if err != nil {
		return nil, err
	}
	return os.FindProcess(pid)
}

func (tk *TestKeeper) SignalPG(sig os.Signal) error {
	p, err := tk.GetPGProcess()
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

func (tk *TestKeeper) isInRecovery() (bool, error) {
	rows, err := tk.Query("SELECT pg_is_in_recovery from pg_is_in_recovery()")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var isInRecovery bool
		if err := rows.Scan(&isInRecovery); err != nil {
			return false, err
		}
		if isInRecovery {
			return true, nil
		}
		return false, nil
	}
	return false, fmt.Errorf("no rows returned")
}

func (tk *TestKeeper) WaitDBRole(r common.Role, ptk *TestKeeper, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		time.Sleep(sleepInterval)
		// when the cluster is in standby mode also the master db is a standby
		// so we cannot just check if the keeper is in recovery but have to
		// check if the primary_conninfo points to the primary db or to the
		// cluster master
		if ptk == nil {
			ok, err := tk.isInRecovery()
			if err != nil {
				continue
			}
			if !ok && r == common.RoleMaster {
				return nil
			}
			if ok && r == common.RoleStandby {
				return nil
			}
		} else {
			ok, err := tk.isInRecovery()
			if err != nil {
				continue
			}
			if !ok {
				continue
			}
			// TODO(sgotti) get this information from the running instance instead than from
			// recovery.conf to be really sure it's applied
			conninfo, err := tk.GetPrimaryConninfo()
			if err != nil {
				continue
			}
			if conninfo["host"] == ptk.pgListenAddress && conninfo["port"] == ptk.pgPort {
				if r == common.RoleMaster {
					return nil
				}
			} else {
				if r == common.RoleStandby {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("timeout")
}

func (tk *TestKeeper) GetPGParameters() (common.Parameters, error) {
	return GetPGParameters(tk)
}

type CheckFunc func(time.Duration) error

func waitChecks(timeout time.Duration, fns ...CheckFunc) error {
	end := make(chan error)
	fnc := len(fns)
	for _, fn := range fns {
		go func(fn CheckFunc, end chan error) {
			end <- fn(timeout)
		}(fn, end)
	}

	c := 0
	for c < fnc {
		err := <-end
		if err != nil {
			return err
		}
		c++
	}
	return nil
}

type TestSentinel struct {
	t *testing.T
	Process
}

func NewTestSentinel(t *testing.T, dir string, clusterName string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestSentinel, error) {
	u := uuid.NewV4()
	uid := fmt.Sprintf("%x", u[:4])

	args := []string{}
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))
	args = append(args, fmt.Sprintf("--store-backend=%s", storeBackend))
	args = append(args, fmt.Sprintf("--store-endpoints=%s", storeEndpoints))
	if os.Getenv("DEBUG") != "" {
		args = append(args, "--debug")
	}
	args = append(args, a...)

	bin := os.Getenv("STSENTINEL_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing STSENTINEL_BIN env")
	}
	ts := &TestSentinel{
		t: t,
		Process: Process{
			t:    t,
			uid:  uid,
			name: "sentinel",
			bin:  bin,
			args: args,
		},
	}
	return ts, nil
}

type TestProxy struct {
	t *testing.T
	Process
	listenAddress string
	port          string
	db            *sql.DB
}

func NewTestProxy(t *testing.T, dir string, clusterName, pgSUUsername, pgSUPassword string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestProxy, error) {
	u := uuid.NewV4()
	uid := fmt.Sprintf("%x", u[:4])

	listenAddress, port, err := getFreePort(true, false)
	if err != nil {
		return nil, err
	}

	args := []string{}
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))
	args = append(args, fmt.Sprintf("--listen-address=%s", listenAddress))
	args = append(args, fmt.Sprintf("--port=%s", port))
	args = append(args, fmt.Sprintf("--store-backend=%s", storeBackend))
	args = append(args, fmt.Sprintf("--store-endpoints=%s", storeEndpoints))
	if os.Getenv("DEBUG") != "" {
		args = append(args, "--debug")
	}
	args = append(args, a...)

	connParams := pg.ConnParams{
		"user":     pgSUUsername,
		"password": pgSUPassword,
		"host":     listenAddress,
		"port":     port,
		"dbname":   "postgres",
		"sslmode":  "disable",
	}

	connString := connParams.ConnString()
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	bin := os.Getenv("STPROXY_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing STPROXY_BIN env")
	}
	tp := &TestProxy{
		t: t,
		Process: Process{
			t:    t,
			uid:  uid,
			name: "proxy",
			bin:  bin,
			args: args,
		},
		listenAddress: listenAddress,
		port:          port,
		db:            db,
	}
	return tp, nil
}

func (tp *TestProxy) WaitListening(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := net.DialTimeout("tcp", net.JoinHostPort(tp.listenAddress, tp.port), timeout-time.Now().Sub(start))
		if err == nil {
			return nil
		}
		tp.t.Logf("tp: %v, error: %v", tp.uid, err)
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func (tp *TestProxy) CheckListening() bool {
	_, err := net.Dial("tcp", net.JoinHostPort(tp.listenAddress, tp.port))
	if err != nil {
		return false
	}
	return true
}
func (tp *TestProxy) WaitNotListening(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := net.DialTimeout("tcp", net.JoinHostPort(tp.listenAddress, tp.port), timeout-time.Now().Sub(start))
		if err != nil {
			return nil
		}
		tp.t.Logf("tp: %v, error: %v", tp.uid, err)
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func (tp *TestProxy) Exec(query string, args ...interface{}) (sql.Result, error) {
	res, err := tp.db.Exec(query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tp *TestProxy) Query(query string, args ...interface{}) (*sql.Rows, error) {
	res, err := tp.db.Query(query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tp *TestProxy) GetPGParameters() (common.Parameters, error) {
	return GetPGParameters(tp)
}

func (tp *TestProxy) WaitRightMaster(tk *TestKeeper, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		pgParameters, err := GetPGParameters(tp)
		if err != nil {
			goto end
		}
		if pgParameters["port"] == tk.pgPort {
			return nil
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func StolonCtl(clusterName string, storeBackend store.Backend, storeEndpoints string, a ...string) error {
	args := []string{}
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))
	args = append(args, fmt.Sprintf("--store-backend=%s", storeBackend))
	args = append(args, fmt.Sprintf("--store-endpoints=%s", storeEndpoints))
	args = append(args, a...)

	bin := os.Getenv("STCTL_BIN")
	if bin == "" {
		return fmt.Errorf("missing STCTL_BIN env")
	}
	cmd := exec.Command(bin, args...)
	return cmd.Run()
}

type TestStore struct {
	t *testing.T
	Process
	listenAddress string
	port          string
	store         kvstore.Store
	storeBackend  store.Backend
}

func NewTestStore(t *testing.T, dir string, a ...string) (*TestStore, error) {
	storeBackend := store.Backend(os.Getenv("STOLON_TEST_STORE_BACKEND"))
	switch storeBackend {
	case store.CONSUL:
		return NewTestConsul(t, dir, a...)
	case store.ETCD:
		return NewTestEtcd(t, dir, a...)
	}
	return nil, fmt.Errorf("wrong store backend")
}

func NewTestEtcd(t *testing.T, dir string, a ...string) (*TestStore, error) {
	u := uuid.NewV4()
	uid := fmt.Sprintf("%x", u[:4])

	dataDir := filepath.Join(dir, fmt.Sprintf("etcd%s", uid))

	listenAddress, port, err := getFreePort(true, false)
	if err != nil {
		return nil, err
	}
	listenAddress2, port2, err := getFreePort(true, false)
	if err != nil {
		return nil, err
	}

	args := []string{}
	args = append(args, fmt.Sprintf("--name=%s", uid))
	args = append(args, fmt.Sprintf("--data-dir=%s", dataDir))
	args = append(args, fmt.Sprintf("--listen-client-urls=http://%s:%s", listenAddress, port))
	args = append(args, fmt.Sprintf("--advertise-client-urls=http://%s:%s", listenAddress, port))
	args = append(args, fmt.Sprintf("--listen-peer-urls=http://%s:%s", listenAddress2, port2))
	args = append(args, fmt.Sprintf("--initial-advertise-peer-urls=http://%s:%s", listenAddress2, port2))
	args = append(args, fmt.Sprintf("--initial-cluster=%s=http://%s:%s", uid, listenAddress2, port2))
	args = append(args, a...)

	storeEndpoints := fmt.Sprintf("%s:%s", listenAddress, port)

	storeConfig := store.Config{
		Backend:   store.ETCD,
		Endpoints: storeEndpoints,
	}
	kvstore, err := store.NewStore(storeConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	bin := os.Getenv("ETCD_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing ETCD_BIN env")
	}
	tstore := &TestStore{
		t: t,
		Process: Process{
			t:    t,
			uid:  uid,
			name: "etcd",
			bin:  bin,
			args: args,
		},
		listenAddress: listenAddress,
		port:          port,
		store:         kvstore,
		storeBackend:  store.ETCD,
	}
	return tstore, nil
}

func NewTestConsul(t *testing.T, dir string, a ...string) (*TestStore, error) {
	u := uuid.NewV4()
	uid := fmt.Sprintf("%x", u[:4])

	dataDir := filepath.Join(dir, fmt.Sprintf("consul%s", uid))

	listenAddress, portHTTP, err := getFreePort(true, false)
	if err != nil {
		return nil, err
	}
	_, portRPC, err := getFreePort(true, false)
	if err != nil {
		return nil, err
	}
	_, portSerfLan, err := getFreePort(true, true)
	if err != nil {
		return nil, err
	}
	_, portSerfWan, err := getFreePort(true, true)
	if err != nil {
		return nil, err
	}
	_, portServer, err := getFreePort(true, false)
	if err != nil {
		return nil, err
	}

	f, err := ioutil.TempFile(dir, "consul.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	f.WriteString(fmt.Sprintf(`{
		"ports": {
			"dns": -1,
			"http": %s,
			"rpc": %s,
			"serf_lan": %s,
			"serf_wan": %s,
			"server": %s
		}
	}`, portHTTP, portRPC, portSerfLan, portSerfWan, portServer))

	args := []string{}
	args = append(args, "agent")
	args = append(args, "-server")
	args = append(args, fmt.Sprintf("-config-file=%s", f.Name()))
	args = append(args, fmt.Sprintf("-data-dir=%s", dataDir))
	args = append(args, fmt.Sprintf("-bind=%s", listenAddress))
	args = append(args, fmt.Sprintf("-advertise=%s", listenAddress))
	args = append(args, "-bootstrap-expect=1")
	args = append(args, a...)

	storeEndpoints := fmt.Sprintf("%s:%s", listenAddress, portHTTP)

	storeConfig := store.Config{
		Backend:   store.CONSUL,
		Endpoints: storeEndpoints,
	}
	kvstore, err := store.NewStore(storeConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	bin := os.Getenv("CONSUL_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing CONSUL_BIN env")
	}
	ts := &TestStore{
		t: t,
		Process: Process{
			t:    t,
			uid:  uid,
			name: "consul",
			bin:  bin,
			args: args,
		},
		listenAddress: listenAddress,
		port:          portHTTP,
		store:         kvstore,
		storeBackend:  store.CONSUL,
	}
	return ts, nil
}

func (ts *TestStore) WaitUp(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := ts.store.Get("anykey")
		ts.t.Logf("err: %v", err)
		if err != nil && err == kvstore.ErrKeyNotFound {
			return nil
		}
		if err == nil {
			return nil
		}
		time.Sleep(sleepInterval)
	}

	return fmt.Errorf("timeout")
}

func (ts *TestStore) WaitDown(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := ts.store.Get("anykey")
		if err != nil && err != kvstore.ErrKeyNotFound {
			return nil
		}
		time.Sleep(sleepInterval)
	}

	return fmt.Errorf("timeout")
}

func WaitClusterDataWithMaster(e *store.StoreManager, timeout time.Duration) (string, error) {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}
		if cd.Cluster.Status.Phase == cluster.ClusterPhaseNormal && cd.Cluster.Status.Master != "" {
			return cd.DBs[cd.Cluster.Status.Master].Spec.KeeperUID, nil
		}
	end:
		time.Sleep(sleepInterval)
	}
	return "", fmt.Errorf("timeout")
}

func WaitClusterDataMaster(master string, e *store.StoreManager, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}
		if cd.Cluster.Status.Phase == cluster.ClusterPhaseNormal && cd.Cluster.Status.Master != "" {
			if cd.DBs[cd.Cluster.Status.Master].Spec.KeeperUID == master {
				return nil
			}
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func WaitClusterDataKeeperInitialized(keeperUID string, e *store.StoreManager, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}
		// Check for db on keeper to be initialized
		for _, db := range cd.DBs {
			if db.Spec.KeeperUID == keeperUID {
				if db.Status.CurrentGeneration >= cluster.InitialGeneration {
					return nil
				}
			}
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

// WaitClusterDataSynchronousStandbys waits for:
// * synchronous standby defined in masterdb spec
// * synchronous standby reported from masterdb status
func WaitClusterDataSynchronousStandbys(synchronousStandbys []string, e *store.StoreManager, timeout time.Duration) error {
	sort.Sort(sort.StringSlice(synchronousStandbys))
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}
		if cd.Cluster.Status.Phase == cluster.ClusterPhaseNormal && cd.Cluster.Status.Master != "" {
			masterDB := cd.DBs[cd.Cluster.Status.Master]
			// get keepers for db spec synchronousStandbys
			keepersUIDs := []string{}
			for _, dbUID := range masterDB.Spec.SynchronousStandbys {
				db, ok := cd.DBs[dbUID]
				if ok {
					keepersUIDs = append(keepersUIDs, db.Spec.KeeperUID)
				}
			}
			sort.Sort(sort.StringSlice(keepersUIDs))
			if !reflect.DeepEqual(synchronousStandbys, keepersUIDs) {
				goto end
			}

			// get keepers for db status synchronousStandbys
			keepersUIDs = []string{}
			for _, dbUID := range masterDB.Status.SynchronousStandbys {
				db, ok := cd.DBs[dbUID]
				if ok {
					keepersUIDs = append(keepersUIDs, db.Spec.KeeperUID)
				}
			}
			sort.Sort(sort.StringSlice(keepersUIDs))
			if !reflect.DeepEqual(synchronousStandbys, keepersUIDs) {
				goto end
			}
			return nil
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func WaitClusterPhase(e *store.StoreManager, phase cluster.ClusterPhase, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}
		if cd.Cluster.Status.Phase == phase {
			return nil
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func WaitNumDBs(e *store.StoreManager, n int, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}
		if len(cd.DBs) == n {
			return nil
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func WaitStandbyKeeper(e *store.StoreManager, keeperUID string, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}

		for _, db := range cd.DBs {
			if db.UID == cd.Cluster.Status.Master {
				continue
			}
			if db.Spec.KeeperUID == keeperUID && db.Spec.Role == common.RoleStandby {
				return nil
			}
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func WaitClusterDataKeepers(keepersUIDs []string, e *store.StoreManager, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}
		if len(keepersUIDs) != len(cd.Keepers) {
			goto end
		}
		// Check for db on keeper to be initialized
		for _, keeper := range cd.Keepers {
			if !util.StringInSlice(keepersUIDs, keeper.UID) {
				goto end
			}
		}
		return nil
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

// WaitClusterSyncedXLogPos waits for all the specified keepers to have the same
// reported XLogPos
func WaitClusterSyncedXLogPos(keepersUIDs []string, e *store.StoreManager, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		c := 0
		curXLogPos := uint64(0)
		cd, _, err := e.GetClusterData()
		if err != nil || cd == nil {
			goto end
		}
		// Check for db on keeper to be initialized
		for _, keeper := range cd.Keepers {
			if !util.StringInSlice(keepersUIDs, keeper.UID) {
				continue
			}
			for _, db := range cd.DBs {
				if db.Spec.KeeperUID == keeper.UID {
					if c == 0 {
						curXLogPos = db.Status.XLogPos
					} else {
						if db.Status.XLogPos != curXLogPos {
							goto end
						}
					}
				}
			}
			c++
		}
		if c == len(keepersUIDs) {
			return nil
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func testFreeTCPPort(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return err
	}
	ln.Close()
	return nil
}

func testFreeUDPPort(port int) error {
	ln, err := net.ListenPacket("udp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return err
	}
	ln.Close()
	return nil
}

// Hack to find a free tcp and udp port
func getFreePort(tcp bool, udp bool) (string, string, error) {
	portMutex.Lock()
	defer portMutex.Unlock()

	if !tcp && !udp {
		return "", "", fmt.Errorf("at least one of tcp or udp port shuld be required")
	}
	localhostIP, err := net.ResolveIPAddr("ip", "localhost")
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve ip addr: %v", err)
	}
	for {
		curPort++
		if curPort > MaxPort {
			return "", "", fmt.Errorf("all available ports to test have been exausted")
		}
		if tcp {
			if err := testFreeTCPPort(curPort); err != nil {
				continue
			}
		}
		if udp {
			if err := testFreeUDPPort(curPort); err != nil {
				continue
			}
		}
		return localhostIP.IP.String(), strconv.Itoa(curPort), nil
	}
}

func writeClusterSpec(dir string, cs *cluster.ClusterSpec) (string, error) {
	csj, err := json.Marshal(cs)
	if err != nil {
		return "", err
	}
	tmpFile, err := ioutil.TempFile(dir, "initial-cluster-spec.json")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()
	if _, err := tmpFile.Write(csj); err != nil {
		return "", err
	}
	return tmpFile.Name(), nil

}
