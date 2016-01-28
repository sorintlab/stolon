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
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/store"

	kvstore "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/docker/libkv/store"
	_ "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/lib/pq"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/sgotti/gexpect"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
)

type Process struct {
	id   string
	name string
	args []string
	cmd  *gexpect.ExpectSubprocess
	bin  string
}

func (p *Process) start() error {
	if p.cmd != nil {
		panic(fmt.Errorf("%s: cmd not cleanly stopped", p.id))
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
			fmt.Printf("[%s]: %s\n", p.id, scanner.Text())
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
	fmt.Printf("signalling %s %s with %s\n", p.name, p.id, sig)
	if p.cmd == nil {
		panic(fmt.Errorf("p: %s, cmd is empty", p.id))
	}
	return p.cmd.Cmd.Process.Signal(sig)
}

func (p *Process) Kill() {
	fmt.Printf("killing %s %s\n", p.name, p.id)
	if p.cmd == nil {
		panic(fmt.Errorf("p: %s, cmd is empty", p.id))
	}
	p.cmd.Cmd.Process.Signal(os.Kill)
	p.cmd.Wait()
	p.cmd = nil
}

func (p *Process) Stop() {
	fmt.Printf("stopping %s %s\n", p.name, p.id)
	if p.cmd == nil {
		panic(fmt.Errorf("p: %s, cmd is empty", p.id))
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
	Process
	dataDir         string
	listenAddress   string
	port            string
	pgListenAddress string
	pgPort          string
	db              *sql.DB
}

func NewTestKeeperWithID(dir string, id string, clusterName string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestKeeper, error) {
	args := []string{}

	dataDir := filepath.Join(dir, fmt.Sprintf("st%s", id))

	// Hack to find a free tcp port
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	ln2, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	defer ln2.Close()

	listenAddress := ln.Addr().(*net.TCPAddr).IP.String()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	pgListenAddress := ln2.Addr().(*net.TCPAddr).IP.String()
	pgPort := strconv.Itoa(ln2.Addr().(*net.TCPAddr).Port)

	args = append(args, fmt.Sprintf("--id=%s", id))
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))
	args = append(args, fmt.Sprintf("--port=%s", port))
	args = append(args, fmt.Sprintf("--pg-port=%s", pgPort))
	args = append(args, fmt.Sprintf("--data-dir=%s", dataDir))
	args = append(args, fmt.Sprintf("--store-backend=%s", storeBackend))
	args = append(args, fmt.Sprintf("--store-endpoints=%s", storeEndpoints))
	args = append(args, "--debug")
	args = append(args, a...)

	connString := fmt.Sprintf("postgres://%s:%s/postgres?sslmode=disable", pgListenAddress, pgPort)
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	bin := os.Getenv("STKEEPER_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing STKEEPER_BIN env")
	}
	tk := &TestKeeper{
		Process: Process{
			id:   id,
			name: "keeper",
			bin:  bin,
			args: args,
		},
		dataDir:         dataDir,
		listenAddress:   listenAddress,
		port:            port,
		pgListenAddress: pgListenAddress,
		pgPort:          pgPort,
		db:              db,
	}
	return tk, nil
}

func NewTestKeeper(dir string, clusterName string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestKeeper, error) {
	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	return NewTestKeeperWithID(dir, id, clusterName, storeBackend, storeEndpoints, a...)
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
		fmt.Printf("tk: %v, error: %v\n", tk.id, err)
		time.Sleep(2 * time.Second)
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
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func (tk *TestKeeper) WaitUp(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := tk.GetKeeperInfo(timeout - time.Now().Sub(start))
		if err == nil {
			return nil
		}
		fmt.Printf("tk: %v, error: %v\n", tk.id, err)
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func (tk *TestKeeper) WaitDown(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := tk.GetKeeperInfo(timeout - time.Now().Sub(start))
		if err != nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func (tk *TestKeeper) GetKeeperInfo(timeout time.Duration) (*cluster.KeeperInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/info", net.JoinHostPort(tk.listenAddress, tk.port)), nil)
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

func httpDo(ctx context.Context, req *http.Request, tlsConfig *tls.Config, f func(*http.Response, error) error) error {
	tr := &http.Transport{DisableKeepAlives: true, TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: tr}
	c := make(chan error, 1)
	go func() { c <- f(client.Do(req)) }()
	select {
	case <-ctx.Done():
		tr.CancelRequest(req)
		<-c
		return ctx.Err()
	case err := <-c:
		return err
	}
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

func (tk *TestKeeper) IsMaster() (bool, error) {
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
		if !isInRecovery {
			return true, nil
		}
		return false, nil
	}
	return false, fmt.Errorf("no rows returned")
}

func (tk *TestKeeper) WaitRole(r common.Role, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		time.Sleep(2 * time.Second)
		ok, err := tk.IsMaster()
		if err != nil {
			continue
		}
		if ok && r == common.MasterRole {
			return nil
		}
		if !ok && r == common.StandbyRole {
			return nil
		}
	}

	return fmt.Errorf("timeout")
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
	Process

	listenAddress string
	port          string
}

func NewTestSentinel(dir string, clusterName string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestSentinel, error) {
	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	// Hack to find a free tcp port
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()

	listenAddress := ln.Addr().(*net.TCPAddr).IP.String()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	args := []string{}
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))
	args = append(args, fmt.Sprintf("--port=%s", port))
	args = append(args, fmt.Sprintf("--store-backend=%s", storeBackend))
	args = append(args, fmt.Sprintf("--store-endpoints=%s", storeEndpoints))
	args = append(args, "--debug")
	args = append(args, a...)

	bin := os.Getenv("STSENTINEL_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing STSENTINEL_BIN env")
	}
	ts := &TestSentinel{
		Process: Process{
			id:   id,
			name: "sentinel",
			bin:  bin,
			args: args,
		},
		listenAddress: listenAddress,
		port:          port,
	}
	return ts, nil
}

type TestProxy struct {
	Process

	listenAddress string
	port          string
}

func NewTestProxy(dir string, clusterName string, storeBackend store.Backend, storeEndpoints string, a ...string) (*TestProxy, error) {
	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	// Hack to find a free tcp port
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()

	listenAddress := ln.Addr().(*net.TCPAddr).IP.String()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	args := []string{}
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))
	args = append(args, fmt.Sprintf("--port=%s", port))
	args = append(args, fmt.Sprintf("--store-backend=%s", storeBackend))
	args = append(args, fmt.Sprintf("--store-endpoints=%s", storeEndpoints))
	args = append(args, "--debug")
	args = append(args, a...)

	bin := os.Getenv("STPROXY_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing STPROXY_BIN env")
	}
	tp := &TestProxy{
		Process: Process{
			id:   id,
			name: "proxy",
			bin:  bin,
			args: args,
		},
		listenAddress: listenAddress,
		port:          port,
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
		fmt.Printf("tp: %v, error: %v\n", tp.id, err)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout")
}

func (tp *TestProxy) WaitNotListening(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := net.DialTimeout("tcp", net.JoinHostPort(tp.listenAddress, tp.port), timeout-time.Now().Sub(start))
		if err != nil {
			return nil
		}
		fmt.Printf("tp: %v, error: %v\n", tp.id, err)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout")
}

type TestStore struct {
	Process

	listenAddress string
	port          string
	store         kvstore.Store
	storeBackend  store.Backend
}

func NewTestStore(dir string, a ...string) (*TestStore, error) {
	storeBackend := store.Backend(os.Getenv("STOLON_TEST_STORE_BACKEND"))
	switch storeBackend {
	case store.CONSUL:
		return NewTestConsul(dir, a...)
	case store.ETCD:
		return NewTestEtcd(dir, a...)
	}
	return nil, fmt.Errorf("wrong store backend")
}

func NewTestEtcd(dir string, a ...string) (*TestStore, error) {
	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	dataDir := filepath.Join(dir, "etcd")

	// Hack to find a free tcp port
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	ln2, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	defer ln2.Close()

	listenAddress := ln.Addr().(*net.TCPAddr).IP.String()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	listenAddress2 := ln2.Addr().(*net.TCPAddr).IP.String()
	port2 := strconv.Itoa(ln2.Addr().(*net.TCPAddr).Port)

	args := []string{}
	args = append(args, fmt.Sprintf("--name=%s", id))
	args = append(args, fmt.Sprintf("--data-dir=%s", dataDir))
	args = append(args, fmt.Sprintf("--listen-client-urls=http://%s:%s", listenAddress, port))
	args = append(args, fmt.Sprintf("--advertise-client-urls=http://%s:%s", listenAddress, port))
	args = append(args, fmt.Sprintf("--listen-peer-urls=http://%s:%s", listenAddress2, port2))
	args = append(args, fmt.Sprintf("--initial-advertise-peer-urls=http://%s:%s", listenAddress2, port2))
	args = append(args, fmt.Sprintf("--initial-cluster=%s=http://%s:%s", id, listenAddress2, port2))
	args = append(args, a...)

	storeEndpoints := fmt.Sprintf("%s:%s", listenAddress, port)

	kvstore, err := store.NewStore(store.ETCD, storeEndpoints)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	bin := os.Getenv("ETCD_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing ETCD_BIN env")
	}
	te := &TestStore{
		Process: Process{
			id:   id,
			name: "etcd",
			bin:  bin,
			args: args,
		},
		listenAddress: listenAddress,
		port:          port,
		store:         kvstore,
		storeBackend:  store.ETCD,
	}
	return te, nil
}

func NewTestConsul(dir string, a ...string) (*TestStore, error) {
	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	dataDir := filepath.Join(dir, "consul")

	listenAddress, portHTTP, err := getFreeTCPPort()
	if err != nil {
		return nil, err
	}
	_, portRPC, err := getFreeTCPPort()
	if err != nil {
		return nil, err
	}
	_, portSerfLan, err := getFreeTCPUDPPort()
	if err != nil {
		return nil, err
	}
	_, portSerfWan, err := getFreeTCPUDPPort()
	if err != nil {
		return nil, err
	}
	_, portServer, err := getFreeTCPPort()
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

	kvstore, err := store.NewStore(store.CONSUL, storeEndpoints)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	bin := os.Getenv("CONSUL_BIN")
	if bin == "" {
		return nil, fmt.Errorf("missing CONSUL_BIN env")
	}
	te := &TestStore{
		Process: Process{
			id:   id,
			name: "consul",
			bin:  bin,
			args: args,
		},
		listenAddress: listenAddress,
		port:          portHTTP,
		store:         kvstore,
		storeBackend:  store.CONSUL,
	}
	return te, nil
}

func (te *TestStore) WaitUp(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := te.store.Get("anykey")
		fmt.Printf("err: %v\n", err)
		if err != nil && err == kvstore.ErrKeyNotFound {
			return nil
		}
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func (te *TestStore) WaitDown(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := te.store.Get("anykey")
		if err != nil && err != kvstore.ErrKeyNotFound {
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func WaitClusterViewWithMaster(e *store.StoreManager, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cv, _, err := e.GetClusterView()
		if err != nil {
			goto end
		}
		if cv != nil {
			if cv.Master != "" {
				return nil
			}
		}
	end:
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout")
}

func WaitClusterViewMaster(master string, e *store.StoreManager, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cv, _, err := e.GetClusterView()
		if err != nil {
			goto end
		}
		if cv != nil {
			if cv.Master == master {
				return nil
			}
		}
	end:
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout")
}

func WaitClusterInitialized(e *store.StoreManager, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		cv, _, err := e.GetClusterView()
		if err != nil {
			goto end
		}
		if cv != nil {
			if cv.Version > 0 {
				return nil
			}
		}
	end:
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout")
}

// Hack to find a free tcp port
func getFreeTCPPort() (string, string, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", "", err
	}
	defer ln.Close()

	listenAddress := ln.Addr().(*net.TCPAddr).IP.String()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	return listenAddress, port, nil
}

// Hack to find a free tcp and udp port with same number
func getFreeTCPUDPPort() (string, string, error) {
	for c := 0; c < 10; c++ {
		ln, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			return "", "", err
		}

		listenAddress := ln.Addr().(*net.TCPAddr).IP.String()
		port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

		ln.Close()

		lnudp, err := net.ListenPacket("udp", fmt.Sprintf("%s:%s", listenAddress, port))
		if err != nil {
			continue
		}
		lnudp.Close()
		return listenAddress, port, nil
	}
	return "", "", fmt.Errorf("failed to find free tcp and udp port")
}
