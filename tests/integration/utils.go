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
	tk := &TestKeeper{
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
	return tk, nil
}

func NewTestKeeper(dir string, clusterName string) (*TestKeeper, error) {
	u := uuid.NewV4()
	id := fmt.Sprintf("%x", u[:4])

	return NewTestKeeperWithID(dir, id, clusterName)
}

func (tk *TestKeeper) Start() error {
	if tk.cmd != nil {
		panic(fmt.Errorf("tk: %s, cmd not cleanly stopped", tk.id))
	}
	tk.cmd = exec.Command(tk.keeperBin, tk.args...)
	stdoutPipe, err := tk.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			fmt.Printf("[%s]: %s\n", tk.id, scanner.Text())
		}
	}()

	stderrPipe, err := tk.cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			fmt.Printf("[%s]: %s\n", tk.id, scanner.Text())
		}
	}()
	err = tk.cmd.Start()
	if err != nil {
		return err
	}
	return nil
}

func (tk *TestKeeper) Signal(sig os.Signal) error {
	fmt.Printf("signalling keeper: %s with %s\n", tk.id, sig)
	if tk.cmd == nil {
		panic(fmt.Errorf("tk: %s, cmd is empty", tk.id))
	}
	return tk.cmd.Process.Signal(sig)
}

func (tk *TestKeeper) Kill() {
	fmt.Printf("killing keeper: %s\n", tk.id)
	if tk.cmd == nil {
		panic(fmt.Errorf("tk: %s, cmd is empty", tk.id))
	}
	tk.cmd.Process.Signal(os.Kill)
	tk.cmd.Wait()
	tk.cmd = nil
}

func (tk *TestKeeper) Stop() {
	fmt.Printf("stopping keeper: %s\n", tk.id)
	if tk.cmd == nil {
		panic(fmt.Errorf("tk: %s, cmd is empty", tk.id))
	}
	tk.cmd.Process.Signal(os.Interrupt)
	tk.cmd.Wait()
	tk.cmd = nil
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

func (tk *TestKeeper) WaitProcess(timeout time.Duration) error {
	timeoutCh := time.NewTimer(timeout).C
	endCh := make(chan error)
	go func() {
		err := tk.cmd.Wait()
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
