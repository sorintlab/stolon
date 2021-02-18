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
	"context"
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

	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/common"
	pg "github.com/sorintlab/stolon/internal/postgresql"
	"github.com/sorintlab/stolon/internal/store"
	"github.com/sorintlab/stolon/internal/util"

	_ "github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/sgotti/gexpect"
)

const (
	sleepInterval = 500 * time.Millisecond

	MinPort = 2048
	MaxPort = 16384
)

var (
	defaultPGParameters = cluster.PGParameters{"log_destination": "stderr", "logging_collector": "false"}

	defaultStoreTimeout = 1 * time.Second
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

type ReplQuerier interface {
	ReplQuery(query string, args ...interface{}) (*sql.Rows, error)
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

func GetSystemData(q ReplQuerier) (*pg.SystemData, error) {
	rows, err := q.ReplQuery("IDENTIFY_SYSTEM")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var sd pg.SystemData
		var xLogPosLsn string
		var unused *string
		if err = rows.Scan(&sd.SystemID, &sd.TimelineID, &xLogPosLsn, &unused); err != nil {
			return nil, err
		}
		sd.XLogPos, err = pg.PGLsnToInt(xLogPosLsn)
		if err != nil {
			return nil, err
		}
		return &sd, nil
	}
	return nil, fmt.Errorf("query returned 0 rows")
}

func GetXLogPos(q ReplQuerier) (uint64, error) {
	// get the current master XLogPos
	systemData, err := GetSystemData(q)
	if err != nil {
		return 0, err
	}
	return systemData.XLogPos, nil
}

// getReplicatinSlots return existing replication slots (also temporary
// replication slots on PostgreSQL > 10)
func getReplicationSlots(q Querier) ([]string, error) {
	replSlots := []string{}

	rows, err := q.Query("select slot_name from pg_replication_slots")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var slotName string
		if err := rows.Scan(&slotName); err != nil {
			return nil, err
		}
		replSlots = append(replSlots, slotName)
	}

	return replSlots, nil
}

/*
// currently unused, keep for future needs

func waitReplicationSlots(q Querier, replSlots []string, timeout time.Duration) error {
	sort.Strings(replSlots)

	start := time.Now()
	curReplSlots := []string{}
	var err error
	for time.Now().Add(-timeout).Before(start) {
		curReplSlots, err := getReplicationSlots(q)
		if err != nil {
			goto end
		}
		sort.Strings(curReplSlots)
		if reflect.DeepEqual(replSlots, curReplSlots) {
			return nil
		}
	end:
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for replSlots %v, got: %v, last err: %v", replSlots, curReplSlots, err)
}
*/

func waitStolonReplicationSlots(q Querier, replSlots []string, timeout time.Duration) error {
	// prefix with stolon_
	for i, slot := range replSlots {
		replSlots[i] = common.StolonName(slot)
	}
	sort.Strings(replSlots)

	start := time.Now()
	var curReplSlots []string
	var err error
	for time.Now().Add(-timeout).Before(start) {
		allReplSlots, err := getReplicationSlots(q)
		if err != nil {
			goto end
		}
		curReplSlots = []string{}
		for _, s := range allReplSlots {
			if common.IsStolonName(s) {
				curReplSlots = append(curReplSlots, s)
			}
		}
		sort.Strings(curReplSlots)
		if reflect.DeepEqual(replSlots, curReplSlots) {
			return nil
		}
	end:
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for replSlots %v, got: %v, last err: %v", replSlots, curReplSlots, err)
}

func waitNotStolonReplicationSlots(q Querier, replSlots []string, timeout time.Duration) error {
	sort.Strings(replSlots)

	start := time.Now()
	var curReplSlots []string
	var err error
	for time.Now().Add(-timeout).Before(start) {
		allReplSlots, err := getReplicationSlots(q)
		if err != nil {
			goto end
		}
		curReplSlots = []string{}
		for _, s := range allReplSlots {
			if !common.IsStolonName(s) {
				curReplSlots = append(curReplSlots, s)
			}
		}
		sort.Strings(curReplSlots)
		if reflect.DeepEqual(replSlots, curReplSlots) {
			return nil
		}
	end:
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for replSlots %v, got: %v, last err: %v", replSlots, curReplSlots, err)
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
	_ = p.cmd.Cmd.Process.Signal(os.Kill)
	_ = p.cmd.Wait()
	p.cmd = nil
}

func (p *Process) Stop() {
	p.t.Logf("stopping %s %s", p.name, p.uid)
	if p.cmd == nil {
		panic(fmt.Errorf("p: %s, cmd is empty", p.uid))
	}
	p.cmd.Continue()
	_ = p.cmd.Cmd.Process.Signal(os.Interrupt)
	_ = p.cmd.Wait()
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
	rdb             *sql.DB
}

func NewTestKeeperWithID(t *testing.T, dir, uid, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestKeeper, error) {
	return NewTestKeeperWithIDWithPriority(t, dir, uid, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, storeBackend, storeEndpoints, 0)
}

func NewTestKeeperWithIDWithPriority(t *testing.T, dir, uid, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword string, storeBackend store.Backend, storeEndpoints string, priority int, a ...string) (*TestKeeper, error) {
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
	args = append(args, fmt.Sprintf("--priority=%d", priority))
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

	replConnParams := pg.ConnParams{
		"user":        pgReplUsername,
		"password":    pgReplPassword,
		"host":        pgListenAddress,
		"port":        pgPort,
		"dbname":      "postgres",
		"sslmode":     "disable",
		"replication": "1",
	}

	connString := connParams.ConnString()
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	replConnString := replConnParams.ConnString()
	rdb, err := sql.Open("postgres", replConnString)
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
		rdb:             rdb,
	}
	return tk, nil
}

func NewTestKeeper(t *testing.T, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestKeeper, error) {
	return NewTestKeeperWithPriority(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, storeBackend, storeEndpoints, 0, a...)
}

func NewTestKeeperWithPriority(t *testing.T, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword string, storeBackend store.Backend, storeEndpoints string, priority int, a ...string) (*TestKeeper, error) {
	u := uuid.NewV4()
	uid := fmt.Sprintf("%x", u[:4])

	return NewTestKeeperWithIDWithPriority(t, dir, uid, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, storeBackend, storeEndpoints, priority, a...)
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
	maj, _, err := tk.PGDataVersion()
	if err != nil {
		return nil, err
	}

	confFile := "recovery.conf"
	if maj >= 12 {
		confFile = "postgresql.conf"
	}
	regex := regexp.MustCompile(`\s*primary_conninfo\s*=\s*'(.*)'$`)

	fh, err := os.Open(filepath.Join(tk.dataDir, "postgres", confFile))
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

func (tk *TestKeeper) ReplQuery(query string, args ...interface{}) (*sql.Rows, error) {
	res, err := tk.rdb.Query(query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tk *TestKeeper) SwitchWals(times int) error {
	maj, _, err := tk.PGDataVersion()
	if err != nil {
		return err
	}
	var switchLogFunc string
	if maj < 10 {
		switchLogFunc = "select pg_switch_xlog()"
	} else {
		switchLogFunc = "select pg_switch_wal()"
	}

	_, _ = tk.Exec("DROP TABLE switchwal")
	if _, err := tk.Exec("CREATE TABLE switchwal(ID INT PRIMARY KEY NOT NULL)"); err != nil {
		return err
	}
	// if times > 1 we have to do some transactions or the wal won't switch
	for i := 0; i < times; i++ {
		if _, err := tk.Exec("INSERT INTO switchwal VALUES ($1)", i); err != nil {
			return err
		}
		if _, err := tk.db.Exec(switchLogFunc); err != nil {
			return err
		}
	}
	_, _ = tk.Exec("DROP TABLE switchwal")
	return nil
}

func (tk *TestKeeper) CheckPoint() error {
	_, err := tk.Exec("CHECKPOINT")
	return err
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
	if rows.Next() {
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

/*
// currently unused, keep for future needs

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
*/

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
	rdb           *sql.DB
}

func NewTestProxy(t *testing.T, dir string, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestProxy, error) {
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

	replConnParams := pg.ConnParams{
		"user":        pgReplUsername,
		"password":    pgReplPassword,
		"host":        listenAddress,
		"port":        port,
		"dbname":      "postgres",
		"sslmode":     "disable",
		"replication": "1",
	}

	connString := connParams.ConnString()
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	replConnString := replConnParams.ConnString()
	rdb, err := sql.Open("postgres", replConnString)
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
		rdb:           rdb,
	}
	return tp, nil
}

func (tp *TestProxy) WaitListening(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := net.DialTimeout("tcp", net.JoinHostPort(tp.listenAddress, tp.port), timeout-time.Since(start))
		if err == nil {
			return nil
		}
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func (tp *TestProxy) CheckListening() bool {
	_, err := net.Dial("tcp", net.JoinHostPort(tp.listenAddress, tp.port))
	return err == nil
}

func (tp *TestProxy) WaitNotListening(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := net.DialTimeout("tcp", net.JoinHostPort(tp.listenAddress, tp.port), timeout-time.Since(start))
		if err != nil {
			return nil
		}
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

func (tp *TestProxy) ReplQuery(query string, args ...interface{}) (*sql.Rows, error) {
	res, err := tp.rdb.Query(query, args...)
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

func StolonCtl(t *testing.T, clusterName string, storeBackend store.Backend, storeEndpoints string, a ...string) error {
	args := []string{}
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))
	args = append(args, fmt.Sprintf("--store-backend=%s", storeBackend))
	args = append(args, fmt.Sprintf("--store-endpoints=%s", storeEndpoints))
	args = append(args, a...)

	t.Logf("executing stolonctl, args: %s", args)

	bin := os.Getenv("STCTL_BIN")
	if bin == "" {
		return fmt.Errorf("missing STCTL_BIN env")
	}
	cmd := exec.Command(bin, args...)
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			t.Logf("[%s]: %s", "stolonctl", scanner.Text())
		}
	}()

	return cmd.Run()
}

type TestStore struct {
	t *testing.T
	Process
	listenAddress string
	port          string
	store         store.KVStore
	storeBackend  store.Backend
}

func NewTestStore(t *testing.T, dir string, a ...string) (*TestStore, error) {
	storeBackend := store.Backend(os.Getenv("STOLON_TEST_STORE_BACKEND"))
	switch storeBackend {
	case "consul":
		return NewTestConsul(t, dir, a...)
	case "etcd":
		storeBackend = "etcdv2"
		fallthrough
	case "etcdv2", "etcdv3":
		return NewTestEtcd(t, dir, storeBackend, a...)
	}
	return nil, fmt.Errorf("wrong store backend")
}

func NewTestEtcd(t *testing.T, dir string, backend store.Backend, a ...string) (*TestStore, error) {
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
		Backend:   store.Backend(backend),
		Endpoints: storeEndpoints,
		Timeout:   defaultStoreTimeout,
	}
	kvstore, err := store.NewKVStore(storeConfig)
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
		storeBackend:  backend,
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

	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("consul%s.json", uid)))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.WriteString(fmt.Sprintf(`{
		"ports": {
			"dns": -1,
			"http": %s,
			"serf_lan": %s,
			"serf_wan": %s,
			"server": %s
		}
	}`, portHTTP, portSerfLan, portSerfWan, portServer)); err != nil {
		return nil, err
	}

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
		Timeout:   defaultStoreTimeout,
	}
	kvstore, err := store.NewKVStore(storeConfig)
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
		ctx, cancel := context.WithTimeout(context.Background(), defaultStoreTimeout)
		_, err := ts.store.Get(ctx, "anykey")
		cancel()
		ts.t.Logf("err: %v", err)
		if err != nil && err == store.ErrKeyNotFound {
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
		ctx, cancel := context.WithTimeout(context.Background(), defaultStoreTimeout)
		_, err := ts.store.Get(ctx, "anykey")
		cancel()
		if err != nil && err != store.ErrKeyNotFound {
			return nil
		}
		time.Sleep(sleepInterval)
	}

	return fmt.Errorf("timeout")
}

func WaitClusterDataUpdated(e *store.KVBackedStore, timeout time.Duration) error {
	icd, _, err := e.GetClusterData(context.TODO())
	if err != nil {
		return fmt.Errorf("unexpected err: %v", err)
	}
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
		if err != nil || cd == nil {
			goto end
		}
		if !reflect.DeepEqual(icd, cd) {
			return nil
		}
	end:
		time.Sleep(sleepInterval)
	}
	return fmt.Errorf("timeout")
}

func WaitClusterDataWithMaster(e *store.KVBackedStore, timeout time.Duration) (string, error) {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
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

func WaitClusterDataMaster(master string, e *store.KVBackedStore, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
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

func WaitClusterDataKeeperInitialized(keeperUID string, e *store.KVBackedStore, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
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
func WaitClusterDataSynchronousStandbys(synchronousStandbys []string, e *store.KVBackedStore, timeout time.Duration) error {
	sort.Strings(synchronousStandbys)
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
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
			sort.Strings(keepersUIDs)
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
			sort.Strings(keepersUIDs)
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

func WaitClusterPhase(e *store.KVBackedStore, phase cluster.ClusterPhase, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
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

func WaitNumDBs(e *store.KVBackedStore, n int, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
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

func WaitStandbyKeeper(e *store.KVBackedStore, keeperUID string, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
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

func WaitClusterDataKeepers(keepersUIDs []string, e *store.KVBackedStore, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
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
// reported XLogPos and that it's >= than master XLogPos
func WaitClusterSyncedXLogPos(keepers []*TestKeeper, xLogPos uint64, e *store.KVBackedStore, timeout time.Duration) error {
	keepersUIDs := []string{}
	for _, sk := range keepers {
		keepersUIDs = append(keepersUIDs, sk.uid)
	}

	// check that master and all the keepers XLogPos are the same and >=
	// masterXLogPos
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		c := 0
		curXLogPos := uint64(0)
		cd, _, err := e.GetClusterData(context.TODO())
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
					if db.Status.XLogPos < xLogPos {
						goto end
					}
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

func WaitClusterDataEnabledProxiesNum(e *store.KVBackedStore, n int, timeout time.Duration) error {
	// TODO(sgotti) find a way to retrieve the proxies internally generated uids
	// and check for them instead of relying only on the number of proxies
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cd, _, err := e.GetClusterData(context.TODO())
		if err != nil || cd == nil {
			goto end
		}
		if len(cd.Proxy.Spec.EnabledProxies) == n {
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
