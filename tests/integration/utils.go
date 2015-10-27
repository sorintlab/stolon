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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"

	_ "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/lib/pq"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
)

type TestKeeper struct {
	id        string
	dataDir   string
	keeperBin string
	args      []string
	cmd       *exec.Cmd

	listenAddress   string
	port            string
	pgListenAddress string
	pgPort          string
	db              *sql.DB
}

func NewTestKeeperWithID(dir string, id string, clusterName string) (*TestKeeper, error) {
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
	args = append(args, "--debug")

	connString := fmt.Sprintf("postgres://%s:%s/postgres?sslmode=disable", pgListenAddress, pgPort)
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	keeperBin := os.Getenv("STKEEPER_BIN")
	if keeperBin == "" {
		return nil, fmt.Errorf("missing STKEEPER_BIN env")
	}
	tm := &TestKeeper{
		id:              id,
		dataDir:         dataDir,
		keeperBin:       keeperBin,
		args:            args,
		listenAddress:   listenAddress,
		port:            port,
		pgListenAddress: pgListenAddress,
		pgPort:          pgPort,
		db:              db,
	}
	return tm, nil
}

func NewTestKeeper(dir string, clusterName string) (*TestKeeper, error) {
	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	return NewTestKeeperWithID(dir, id, clusterName)
}

func (tm *TestKeeper) Start() error {
	if tm.cmd != nil {
		panic(fmt.Errorf("tm: %s, cmd not cleanly stopped", tm.id))
	}
	tm.cmd = exec.Command(tm.keeperBin, tm.args...)
	stdoutPipe, err := tm.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			fmt.Printf("[%s]: %s\n", tm.id, scanner.Text())
		}
	}()

	stderrPipe, err := tm.cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			fmt.Printf("[%s]: %s\n", tm.id, scanner.Text())
		}
	}()
	err = tm.cmd.Start()
	if err != nil {
		return err
	}
	return nil
}

func (tm *TestKeeper) Signal(sig os.Signal) error {
	fmt.Printf("signalling keeper: %s with %s\n", tm.id, sig)
	if tm.cmd == nil {
		panic(fmt.Errorf("tm: %s, cmd is empty", tm.id))
	}
	return tm.cmd.Process.Signal(sig)
}

func (tm *TestKeeper) Kill() {
	fmt.Printf("killing keeper: %s\n", tm.id)
	if tm.cmd == nil {
		panic(fmt.Errorf("tm: %s, cmd is empty", tm.id))
	}
	tm.cmd.Process.Signal(os.Kill)
	tm.cmd.Wait()
	tm.cmd = nil
}

func (tm *TestKeeper) Stop() {
	fmt.Printf("stopping keeper: %s\n", tm.id)
	if tm.cmd == nil {
		panic(fmt.Errorf("tm: %s, cmd is empty", tm.id))
	}
	tm.cmd.Process.Signal(os.Interrupt)
	tm.cmd.Wait()
	tm.cmd = nil
}

func (tm *TestKeeper) Exec(query string, args ...interface{}) (sql.Result, error) {
	res, err := tm.db.Exec(query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tm *TestKeeper) Query(query string, args ...interface{}) (*sql.Rows, error) {
	res, err := tm.db.Query(query, args...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tm *TestKeeper) WaitDBUp(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := tm.Exec("select 1")
		if err == nil {
			return nil
		}
		fmt.Printf("tm: %v, error: %v\n", tm.id, err)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func (tm *TestKeeper) WaitDBDown(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := tm.Exec("select 1")
		if err != nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func (tm *TestKeeper) WaitUp(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := tm.GetKeeperInfo(timeout - time.Now().Sub(start))
		if err == nil {
			return nil
		}
		fmt.Printf("tm: %v, error: %v\n", tm.id, err)
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func (tm *TestKeeper) WaitDown(timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		_, err := tm.GetKeeperInfo(timeout - time.Now().Sub(start))
		if err != nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout")
}

func (tm *TestKeeper) GetKeeperInfo(timeout time.Duration) (*cluster.KeeperInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/info", net.JoinHostPort(tm.listenAddress, tm.port)), nil)
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

func (tm *TestKeeper) GetPGProcess() (*os.Process, error) {
	fh, err := os.Open(filepath.Join(tm.dataDir, "postgres/postmaster.pid"))
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

func (tm *TestKeeper) SignalPG(sig os.Signal) error {
	p, err := tm.GetPGProcess()
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

func (tm *TestKeeper) IsMaster() (bool, error) {
	rows, err := tm.Query("SELECT pg_is_in_recovery from pg_is_in_recovery()")
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

func (tm *TestKeeper) WaitRole(r common.Role, timeout time.Duration) error {
	start := time.Now()
	for time.Now().Add(-timeout).Before(start) {
		time.Sleep(2 * time.Second)
		ok, err := tm.IsMaster()
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

func (tm *TestKeeper) WaitProcess(timeout time.Duration) error {
	timeoutCh := time.NewTimer(timeout).C
	endCh := make(chan error)
	go func() {
		err := tm.cmd.Wait()
		endCh <- err
	}()
	select {
	case <-timeoutCh:
		return fmt.Errorf("timeout waiting on process")
	case <-endCh:
		return nil
	}
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
	id          string
	cmd         *exec.Cmd
	sentinelBin string
	args        []string

	listenAddress string
	port          string
}

func NewTestSentinel(dir string, clusterName string) (*TestSentinel, error) {
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
	args = append(args, "--debug")

	sentinelBin := os.Getenv("STSENTINEL_BIN")
	if sentinelBin == "" {
		return nil, fmt.Errorf("missing STSENTINEL_BIN env")
	}
	ts := &TestSentinel{
		id:            id,
		sentinelBin:   sentinelBin,
		args:          args,
		listenAddress: listenAddress,
		port:          port,
	}
	return ts, nil
}

func (ts *TestSentinel) Start() error {
	if ts.cmd != nil {
		panic(fmt.Errorf("ts: %s, cmd not cleanly stopped", ts.id))
	}
	ts.cmd = exec.Command(ts.sentinelBin, ts.args...)
	stdoutPipe, err := ts.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			fmt.Printf("[%s]: %s\n", ts.id, scanner.Text())
		}
	}()

	stderrPipe, err := ts.cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			fmt.Printf("[%s]: %s\n", ts.id, scanner.Text())
		}
	}()
	err = ts.cmd.Start()
	if err != nil {
		return err
	}
	return nil
}

func (ts *TestSentinel) Signal(sig os.Signal) error {
	fmt.Printf("signalling sentinel: %s with %s\n", ts.id, sig)
	if ts.cmd == nil {
		panic(fmt.Errorf("ts: %s, cmd is empty", ts.id))
	}
	return ts.cmd.Process.Signal(sig)
}

func (ts *TestSentinel) Kill() {
	fmt.Printf("killing sentinel: %s\n", ts.id)
	if ts.cmd == nil {
		panic(fmt.Errorf("ts: %s, cmd is empty", ts.id))
	}
	ts.cmd.Process.Signal(os.Kill)
	ts.cmd.Wait()
	ts.cmd = nil
}

func (ts *TestSentinel) Stop() {
	fmt.Printf("stopping sentinel: %s\n", ts.id)
	if ts.cmd == nil {
		panic(fmt.Errorf("ts: %s, cmd is empty", ts.id))
	}
	ts.cmd.Process.Signal(os.Interrupt)
	ts.cmd.Wait()
	ts.cmd = nil
}
