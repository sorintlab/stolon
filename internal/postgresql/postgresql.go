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

package postgresql

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sorintlab/stolon/internal/common"
	slog "github.com/sorintlab/stolon/internal/log"

	_ "github.com/lib/pq"
	"github.com/mitchellh/copystructure"
	"go.uber.org/zap"
)

//go:generate mockgen -destination=../mock/postgresql/postgresql.go -package=mocks -source=$GOFILE

const (
	postgresConf           = "postgresql.conf"
	postgresRecoveryConf   = "recovery.conf"
	postgresStandbySignal  = "standby.signal"
	postgresRecoverySignal = "recovery.signal"
	postgresRecoveryDone   = "recovery.done"
	postgresAutoConf       = "postgresql.auto.conf"
	tmpPostgresConf        = "stolon-temp-postgresql.conf"

	startTimeout = 60 * time.Second
)

var (
	ErrUnknownState = errors.New("unknown postgres state")
)

var log = slog.S()

type PGManager interface {
	GetTimelinesHistory(timeline uint64) ([]*TimelineHistory, error)
}

type Manager struct {
	pgBinPath          string
	dataDir            string
	parameters         common.Parameters
	recoveryOptions    *RecoveryOptions
	hba                []string
	curParameters      common.Parameters
	curRecoveryOptions *RecoveryOptions
	curHba             []string
	localConnParams    ConnParams
	replConnParams     ConnParams
	suAuthMethod       string
	suUsername         string
	suPassword         string
	replAuthMethod     string
	replUsername       string
	replPassword       string
	requestTimeout     time.Duration
}

type RecoveryMode int

const (
	RecoveryModeNone RecoveryMode = iota
	RecoveryModeStandby
	RecoveryModeRecovery
)

type RecoveryOptions struct {
	RecoveryMode       RecoveryMode
	RecoveryParameters common.Parameters
}

func NewRecoveryOptions() *RecoveryOptions {
	return &RecoveryOptions{RecoveryParameters: make(common.Parameters)}
}

func (r *RecoveryOptions) DeepCopy() *RecoveryOptions {
	nr, err := copystructure.Copy(r)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(r, nr) {
		panic("not equal")
	}
	return nr.(*RecoveryOptions)
}

type SystemData struct {
	SystemID   string
	TimelineID uint64
	XLogPos    uint64
}

type TimelineHistory struct {
	TimelineID  uint64
	SwitchPoint uint64
	Reason      string
}

type InitConfig struct {
	Locale        string
	Encoding      string
	DataChecksums bool
}

func SetLogger(l *zap.SugaredLogger) {
	log = l
}

func NewManager(pgBinPath string, dataDir string, localConnParams, replConnParams ConnParams, suAuthMethod, suUsername, suPassword, replAuthMethod, replUsername, replPassword string, requestTimeout time.Duration) *Manager {
	return &Manager{
		pgBinPath:          pgBinPath,
		dataDir:            filepath.Join(dataDir, "postgres"),
		parameters:         make(common.Parameters),
		recoveryOptions:    NewRecoveryOptions(),
		curParameters:      make(common.Parameters),
		curRecoveryOptions: NewRecoveryOptions(),
		replConnParams:     replConnParams,
		localConnParams:    localConnParams,
		suAuthMethod:       suAuthMethod,
		suUsername:         suUsername,
		suPassword:         suPassword,
		replAuthMethod:     replAuthMethod,
		replUsername:       replUsername,
		replPassword:       replPassword,
		requestTimeout:     requestTimeout,
	}
}

func (p *Manager) SetParameters(parameters common.Parameters) {
	p.parameters = parameters
}

func (p *Manager) CurParameters() common.Parameters {
	return p.curParameters
}

func (p *Manager) SetRecoveryOptions(recoveryOptions *RecoveryOptions) {
	if recoveryOptions == nil {
		p.recoveryOptions = NewRecoveryOptions()
		return
	}

	p.recoveryOptions = recoveryOptions
}

func (p *Manager) CurRecoveryOptions() *RecoveryOptions {
	return p.curRecoveryOptions
}

func (p *Manager) SetHba(hba []string) {
	p.hba = hba
}

func (p *Manager) CurHba() []string {
	return p.curHba
}

func (p *Manager) UpdateCurParameters() {
	n, err := copystructure.Copy(p.parameters)
	if err != nil {
		panic(err)
	}
	p.curParameters = n.(common.Parameters)
}

func (p *Manager) UpdateCurRecoveryOptions() {
	p.curRecoveryOptions = p.recoveryOptions.DeepCopy()
}

func (p *Manager) UpdateCurHba() {
	n, err := copystructure.Copy(p.hba)
	if err != nil {
		panic(err)
	}
	p.curHba = n.([]string)
}

func (p *Manager) Init(initConfig *InitConfig) error {
	// ioutil.Tempfile already creates files with 0600 permissions
	pwfile, err := ioutil.TempFile("", "pwfile")
	if err != nil {
		return err
	}
	defer os.Remove(pwfile.Name())
	defer pwfile.Close()

	if _, err = pwfile.WriteString(p.suPassword); err != nil {
		return err
	}

	name := filepath.Join(p.pgBinPath, "initdb")
	cmd := exec.Command(name, "-D", p.dataDir, "-U", p.suUsername)
	if p.suAuthMethod == "md5" {
		cmd.Args = append(cmd.Args, "--pwfile", pwfile.Name())
	}
	log.Debugw("execing cmd", "cmd", cmd)

	if initConfig.Locale != "" {
		cmd.Args = append(cmd.Args, "--locale", initConfig.Locale)
	}
	if initConfig.Encoding != "" {
		cmd.Args = append(cmd.Args, "--encoding", initConfig.Encoding)
	}
	if initConfig.DataChecksums {
		cmd.Args = append(cmd.Args, "--data-checksums")
	}

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		err = fmt.Errorf("error: %v", err)
	}
	// remove the dataDir, so we don't end with an half initialized database
	if err != nil {
		os.RemoveAll(p.dataDir)
		return err
	}
	return nil
}

func (p *Manager) Restore(command string) error {
	var err error
	var cmd *exec.Cmd

	command = expand(command, p.dataDir)

	if err = os.MkdirAll(p.dataDir, 0700); err != nil {
		err = fmt.Errorf("cannot create data dir: %v", err)
		goto out
	}
	cmd = exec.Command("/bin/sh", "-c", command)
	log.Debugw("execing cmd", "cmd", cmd)

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		err = fmt.Errorf("error: %v", err)
		goto out
	}
	// On every error remove the dataDir, so we don't end with an half initialized database
out:
	if err != nil {
		os.RemoveAll(p.dataDir)
		return err
	}
	return nil
}

// StartTmpMerged starts postgres with a conf file different than
// postgresql.conf, including it at the start of the conf if it exists
func (p *Manager) StartTmpMerged() error {
	if err := p.writeConfs(true); err != nil {
		return err
	}
	tmpPostgresConfPath := filepath.Join(p.dataDir, tmpPostgresConf)

	return p.start("-c", fmt.Sprintf("config_file=%s", tmpPostgresConfPath))
}

func (p *Manager) Start() error {
	if err := p.writeConfs(false); err != nil {
		return err
	}
	return p.start()
}

// start starts the instance. A success means that the instance has been
// successfully started BUT doesn't mean that the instance is ready to accept
// connections (i.e. it's waiting for some missing wals etc...).
// Note that also on error an instance may still be active and, if needed,
// should be manually stopped calling Stop.
func (p *Manager) start(args ...string) error {
	// pg_ctl for postgres < 10 with -w will exit after the timeout and return 0
	// also if the instance isn't ready to accept connections, while for
	// postgres >= 10 it will return a non 0 exit code making it impossible to
	// distinguish between problems starting an instance (i.e. wrong parameters)
	// or an instance started but not ready to accept connections.
	// To work with all the versions and since we want to distinguish between a
	// failed start and a started but not ready instance we are forced to not
	// use pg_ctl and write part of its logic here (I hate to do this).

	// A difference between directly calling postgres instead of pg_ctl is that
	// the instance parent is the keeper instead of the defined system reaper
	// (since pg_ctl forks and then exits leaving the postmaster orphaned).

	if err := p.createPostgresqlAutoConf(); err != nil {
		return err
	}

	log.Infow("starting database")
	name := filepath.Join(p.pgBinPath, "postgres")
	args = append([]string{"-D", p.dataDir, "-c", "unix_socket_directories=" + common.PgUnixSocketDirectories}, args...)
	cmd := exec.Command(name, args...)
	log.Debugw("execing cmd", "cmd", cmd)
	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error: %v", err)
	}

	// execute child wait in a goroutine so we'll wait for it to exit without
	// leaving zombie childs
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	pid := cmd.Process.Pid

	// Wait for the correct pid file to appear or for the process to exit
	ok := false
	start := time.Now()
	for time.Since(start) < startTimeout {
		fh, err := os.Open(filepath.Join(p.dataDir, "postmaster.pid"))
		if err == nil {
			scanner := bufio.NewScanner(fh)
			scanner.Split(bufio.ScanLines)
			if scanner.Scan() {
				fpid := scanner.Text()
				if fpid == strconv.Itoa(pid) {
					ok = true
					fh.Close()
					break
				}
			}
		}
		fh.Close()

		select {
		case <-exited:
			return fmt.Errorf("postgres exited unexpectedly")
		default:
		}

		time.Sleep(200 * time.Millisecond)
	}

	if !ok {
		return fmt.Errorf("instance still starting")
	}

	p.UpdateCurParameters()
	p.UpdateCurRecoveryOptions()
	p.UpdateCurHba()

	return nil
}

// Stop tries to stop an instance. An error will be returned if the instance isn't started, stop fails or
// times out (60 second).
func (p *Manager) Stop(fast bool) error {
	log.Infow("stopping database")
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "stop", "-w", "-D", p.dataDir, "-o", "-c unix_socket_directories="+common.PgUnixSocketDirectories)
	if fast {
		cmd.Args = append(cmd.Args, "-m", "fast")
	}
	log.Debugw("execing cmd", "cmd", cmd)

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: %v", err)
	}
	return nil
}

func (p *Manager) IsStarted() (bool, error) {
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "status", "-D", p.dataDir, "-o", "-c unix_socket_directories="+common.PgUnixSocketDirectories)
	_, err := cmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			status := cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
			if status == 3 {
				return false, nil
			}
			if status == 4 {
				return false, ErrUnknownState
			}
		}
		return false, fmt.Errorf("cannot get instance state: %v", err)
	}
	return true, nil
}

func (p *Manager) Reload() error {
	log.Infow("reloading database configuration")

	if err := p.writeConfs(false); err != nil {
		return err
	}

	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "reload", "-D", p.dataDir, "-o", "-c unix_socket_directories="+common.PgUnixSocketDirectories)
	log.Debugw("execing cmd", "cmd", cmd)

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: %v", err)
	}

	p.UpdateCurParameters()
	p.UpdateCurRecoveryOptions()
	p.UpdateCurHba()

	return nil
}

// StopIfStarted checks if the instance is started, then calls stop and
// then check if the instance is really stopped
func (p *Manager) StopIfStarted(fast bool) error {
	// Stop will return an error if the instance isn't started, so first check
	// if it's started
	started, err := p.IsStarted()
	if err != nil {
		if err == ErrUnknownState {
			// if IsStarted returns an unknown state error then assume that the
			// instance is stopped
			return nil
		}
		return err
	}
	if !started {
		return nil
	}
	if err = p.Stop(fast); err != nil {
		return err
	}
	started, err = p.IsStarted()
	if err != nil {
		return err
	}
	if started {
		return fmt.Errorf("failed to stop")
	}
	return nil
}

func (p *Manager) Restart(fast bool) error {
	log.Infow("restarting database")
	if err := p.StopIfStarted(fast); err != nil {
		return err
	}
	if err := p.Start(); err != nil {
		return err
	}
	return nil
}

func (p *Manager) WaitReady(timeout time.Duration) error {
	start := time.Now()
	for timeout == 0 || time.Since(start) < timeout {
		if err := p.Ping(); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for db ready")
}

func (p *Manager) WaitRecoveryDone(timeout time.Duration) error {
	maj, _, err := p.BinaryVersion()
	if err != nil {
		return fmt.Errorf("error fetching pg version: %v", err)
	}

	start := time.Now()
	if maj >= 12 {
		for timeout == 0 || time.Since(start) < timeout {
			_, err := os.Stat(filepath.Join(p.dataDir, postgresRecoverySignal))
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if os.IsNotExist(err) {
				return nil
			}
			time.Sleep(1 * time.Second)
		}
	} else {
		for timeout == 0 || time.Since(start) < timeout {
			_, err := os.Stat(filepath.Join(p.dataDir, postgresRecoveryDone))
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if !os.IsNotExist(err) {
				return nil
			}
			time.Sleep(1 * time.Second)
		}
	}

	return fmt.Errorf("timeout waiting for db recovery")
}

func (p *Manager) Promote() error {
	log.Infow("promoting database")

	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "promote", "-w", "-D", p.dataDir)
	log.Debugw("execing cmd", "cmd", cmd)

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: %v", err)
	}

	if err := p.writeConfs(false); err != nil {
		return err
	}

	return nil
}

func (p *Manager) SetupRoles() error {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()

	if p.suUsername == p.replUsername {
		log.Infow("adding replication role to superuser")
		if p.suAuthMethod == "trust" {
			if err := alterPasswordlessRole(ctx, p.localConnParams, []string{"replication"}, p.suUsername); err != nil {
				return fmt.Errorf("error adding replication role to superuser: %v", err)
			}
		} else {
			if err := alterRole(ctx, p.localConnParams, []string{"replication"}, p.suUsername, p.suPassword); err != nil {
				return fmt.Errorf("error adding replication role to superuser: %v", err)
			}
		}
		log.Infow("replication role added to superuser")
	} else {
		// Configure superuser role password if auth method is not trust
		if p.suAuthMethod != "trust" && p.suPassword != "" {
			log.Infow("setting superuser password")
			if err := setPassword(ctx, p.localConnParams, p.suUsername, p.suPassword); err != nil {
				return fmt.Errorf("error setting superuser password: %v", err)
			}
			log.Infow("superuser password set")
		}
		roles := []string{"login", "replication"}
		log.Infow("creating replication role")
		if p.replAuthMethod != "trust" {
			if err := createRole(ctx, p.localConnParams, roles, p.replUsername, p.replPassword); err != nil {
				return fmt.Errorf("error creating replication role: %v", err)
			}
		} else {
			if err := createPasswordlessRole(ctx, p.localConnParams, roles, p.replUsername); err != nil {
				return fmt.Errorf("error creating replication role: %v", err)
			}
		}
		log.Infow("replication role created", "role", p.replUsername)
	}
	return nil
}

func (p *Manager) GetSyncStandbys() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return getSyncStandbys(ctx, p.localConnParams)
}

func (p *Manager) GetReplicationSlots() ([]string, error) {
	maj, _, err := p.PGDataVersion()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return getReplicationSlots(ctx, p.localConnParams, maj)
}

func (p *Manager) CreateReplicationSlot(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return createReplicationSlot(ctx, p.localConnParams, name)
}

func (p *Manager) DropReplicationSlot(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return dropReplicationSlot(ctx, p.localConnParams, name)
}

func (p *Manager) BinaryVersion() (int, int, error) {
	name := filepath.Join(p.pgBinPath, "postgres")
	cmd := exec.Command(name, "-V")
	log.Debugw("execing cmd", "cmd", cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("error: %v, output: %s", err, string(out))
	}

	return ParseBinaryVersion(string(out))
}

func (p *Manager) PGDataVersion() (int, int, error) {
	fh, err := os.Open(filepath.Join(p.dataDir, "PG_VERSION"))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read PG_VERSION: %v", err)
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	scanner.Split(bufio.ScanLines)

	scanner.Scan()

	version := scanner.Text()
	return ParseVersion(version)
}

func (p *Manager) IsInitialized() (bool, error) {
	// List of required files or directories relative to postgres data dir
	// From https://www.postgresql.org/docs/9.4/static/storage-file-layout.html
	// with some additions.
	// TODO(sgotti) when the code to get the current db version is in place
	// also add additinal files introduced by releases after 9.4.
	exists, err := fileExists(filepath.Join(p.dataDir, "PG_VERSION"))
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	maj, _, err := p.PGDataVersion()
	if err != nil {
		return false, err
	}
	requiredFiles := []string{
		"PG_VERSION",
		"base",
		"global",
		"pg_dynshmem",
		"pg_logical",
		"pg_multixact",
		"pg_notify",
		"pg_replslot",
		"pg_serial",
		"pg_snapshots",
		"pg_stat",
		"pg_stat_tmp",
		"pg_subtrans",
		"pg_tblspc",
		"pg_twophase",
		"global/pg_control",
	}
	// in postgres 10 pc_clog has been renamed to pg_xact and pc_xlog has been
	// renamed to pg_wal
	if maj < 10 {
		requiredFiles = append(requiredFiles, []string{
			"pg_clog",
			"pg_xlog",
		}...)
	} else {
		requiredFiles = append(requiredFiles, []string{
			"pg_xact",
			"pg_wal",
		}...)

	}
	for _, f := range requiredFiles {
		exists, err := fileExists(filepath.Join(p.dataDir, f))
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

// GetRole return the current instance role
func (p *Manager) GetRole() (common.Role, error) {
	maj, _, err := p.BinaryVersion()
	if err != nil {
		return "", fmt.Errorf("error fetching pg version: %v", err)
	}

	if maj >= 12 {
		// if standby.signal file exists then consider it as a standby
		_, err := os.Stat(filepath.Join(p.dataDir, postgresStandbySignal))
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("error determining if %q file exists: %v", postgresStandbySignal, err)
		}
		if os.IsNotExist(err) {
			return common.RoleMaster, nil
		}
		return common.RoleStandby, nil
	} else {
		// if recovery.conf file exists then consider it as a standby
		_, err := os.Stat(filepath.Join(p.dataDir, postgresRecoveryConf))
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("error determining if %q file exists: %v", postgresRecoveryConf, err)
		}
		if os.IsNotExist(err) {
			return common.RoleMaster, nil
		}
		return common.RoleStandby, nil
	}
}

func (p *Manager) writeConfs(useTmpPostgresConf bool) error {
	maj, _, err := p.BinaryVersion()
	if err != nil {
		return fmt.Errorf("error fetching pg version: %v", err)
	}

	writeRecoveryParamsInPostgresConf := false
	if maj >= 12 {
		writeRecoveryParamsInPostgresConf = true
	}

	if err := p.writeConf(useTmpPostgresConf, writeRecoveryParamsInPostgresConf); err != nil {
		return fmt.Errorf("error writing %s file: %v", postgresConf, err)
	}
	if err := p.writePgHba(); err != nil {
		return fmt.Errorf("error writing pg_hba.conf file: %v", err)
	}
	if !writeRecoveryParamsInPostgresConf {
		if err := p.writeRecoveryConf(); err != nil {
			return fmt.Errorf("error writing %s file: %v", postgresRecoveryConf, err)
		}
	} else {
		if err := p.writeStandbySignal(); err != nil {
			return fmt.Errorf("error writing %s file: %v", postgresStandbySignal, err)
		}
		if err := p.writeRecoverySignal(); err != nil {
			return fmt.Errorf("error writing %s file: %v", postgresRecoverySignal, err)
		}
	}
	return nil
}

func (p *Manager) writeConf(useTmpPostgresConf, writeRecoveryParams bool) error {
	confFile := postgresConf
	if useTmpPostgresConf {
		confFile = tmpPostgresConf
	}

	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, confFile), 0600,
		func(f io.Writer) error {
			if useTmpPostgresConf {
				// include postgresql.conf if it exists
				_, err := os.Stat(filepath.Join(p.dataDir, postgresConf))
				if err != nil && !os.IsNotExist(err) {
					return err
				}
				if !os.IsNotExist(err) {
					if _, err := f.Write([]byte(fmt.Sprintf("include '%s'\n", postgresConf))); err != nil {
						return err
					}
				}
			}
			for k, v := range p.parameters {
				// Single quotes needs to be doubled
				ev := strings.Replace(v, `'`, `''`, -1)
				if _, err := f.Write([]byte(fmt.Sprintf("%s = '%s'\n", k, ev))); err != nil {
					return err
				}
			}

			if writeRecoveryParams {
				// write recovery parameters only if recoveryMode is not none
				if p.recoveryOptions.RecoveryMode != RecoveryModeNone {
					for n, v := range p.recoveryOptions.RecoveryParameters {
						if _, err := f.Write([]byte(fmt.Sprintf("%s = '%s'\n", n, v))); err != nil {
							return err
						}
					}
				}
			}

			return nil
		})
}

func (p *Manager) writeRecoveryConf() error {
	// write recovery.conf only if recoveryMode is not none
	if p.recoveryOptions.RecoveryMode == RecoveryModeNone {
		return nil
	}

	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, postgresRecoveryConf), 0600,
		func(f io.Writer) error {
			if p.recoveryOptions.RecoveryMode == RecoveryModeStandby {
				if _, err := f.Write([]byte("standby_mode = 'on'\n")); err != nil {
					return err
				}
			}
			for n, v := range p.recoveryOptions.RecoveryParameters {
				if _, err := f.Write([]byte(fmt.Sprintf("%s = '%s'\n", n, v))); err != nil {
					return err
				}
			}
			return nil
		})
}

func (p *Manager) writeStandbySignal() error {
	// write standby.signal only if recoveryMode is standby
	if p.recoveryOptions.RecoveryMode != RecoveryModeStandby {
		return nil
	}

	log.Infof("writing standby signal file")

	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, postgresStandbySignal), 0600,
		func(f io.Writer) error {
			return nil
		})
}

func (p *Manager) writeRecoverySignal() error {
	// write standby.signal only if recoveryMode is recovery
	if p.recoveryOptions.RecoveryMode != RecoveryModeRecovery {
		return nil
	}

	log.Infof("writing recovery signal file")

	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, postgresRecoverySignal), 0600,
		func(f io.Writer) error {
			return nil
		})
}

func (p *Manager) writePgHba() error {
	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, "pg_hba.conf"), 0600,
		func(f io.Writer) error {
			if p.hba != nil {
				for _, e := range p.hba {
					if _, err := f.Write([]byte(e + "\n")); err != nil {
						return err
					}
				}
			}
			return nil
		})
}

// createPostgresqlAutoConf creates postgresql.auto.conf as a symlink to
// /dev/null to block alter systems commands (they'll return an error)
func (p *Manager) createPostgresqlAutoConf() error {
	pgAutoConfPath := filepath.Join(p.dataDir, postgresAutoConf)
	if err := os.Remove(pgAutoConfPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing postgresql.auto.conf file: %v", err)
	}
	if err := os.Symlink("/dev/null", pgAutoConfPath); err != nil {
		return fmt.Errorf("error symlinking postgresql.auto.conf file to /dev/null: %v", err)
	}
	return nil
}

func (p *Manager) SyncFromFollowedPGRewind(followedConnParams ConnParams, password string) error {
	// Remove postgresql.auto.conf since pg_rewind will error if it's a symlink to /dev/null
	pgAutoConfPath := filepath.Join(p.dataDir, postgresAutoConf)
	if err := os.Remove(pgAutoConfPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing postgresql.auto.conf file: %v", err)
	}

	// ioutil.Tempfile already creates files with 0600 permissions
	pgpass, err := ioutil.TempFile("", "pgpass")
	if err != nil {
		return err
	}
	defer os.Remove(pgpass.Name())
	defer pgpass.Close()

	host := followedConnParams.Get("host")
	port := followedConnParams.Get("port")
	user := followedConnParams.Get("user")
	if _, err := pgpass.WriteString(fmt.Sprintf("%s:%s:*:%s:%s\n", host, port, user, password)); err != nil {
		return err
	}

	// Disable synchronous commits. pg_rewind needs to create a
	// temporary table on the master but if synchronous replication is
	// enabled and there're no active standbys it will hang.
	followedConnParams.Set("options", "-c synchronous_commit=off")
	followedConnString := followedConnParams.ConnString()

	log.Infow("running pg_rewind")
	name := filepath.Join(p.pgBinPath, "pg_rewind")
	cmd := exec.Command(name, "--debug", "-D", p.dataDir, "--source-server="+followedConnString)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSFILE=%s", pgpass.Name()))
	log.Debugw("execing cmd", "cmd", cmd)

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: %v", err)
	}
	return nil
}

func (p *Manager) SyncFromFollowed(followedConnParams ConnParams, replSlot string) error {
	fcp := followedConnParams.Copy()

	// ioutil.Tempfile already creates files with 0600 permissions
	pgpass, err := ioutil.TempFile("", "pgpass")
	if err != nil {
		return err
	}
	defer os.Remove(pgpass.Name())
	defer pgpass.Close()

	host := fcp.Get("host")
	port := fcp.Get("port")
	user := fcp.Get("user")
	password := fcp.Get("password")
	if _, err = pgpass.WriteString(fmt.Sprintf("%s:%s:*:%s:%s\n", host, port, user, password)); err != nil {
		return err
	}

	// Remove password from the params passed to pg_basebackup
	fcp.Del("password")

	// Disable synchronous commits. pg_basebackup calls
	// pg_start_backup()/pg_stop_backup() on the master but if synchronous
	// replication is enabled and there're no active standbys they will hang.
	fcp.Set("options", "-c synchronous_commit=off")
	followedConnString := fcp.ConnString()

	log.Infow("running pg_basebackup")
	name := filepath.Join(p.pgBinPath, "pg_basebackup")
	args := []string{"-R", "-v", "-P", "-Xs", "-D", p.dataDir, "-d", followedConnString}
	if replSlot != "" {
		args = append(args, "--slot", replSlot)
	}
	cmd := exec.Command(name, args...)

	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSFILE=%s", pgpass.Name()))
	log.Debugw("execing cmd", "cmd", cmd)

	// Pipe pg_basebackup's stderr to our stderr.
	// We do this indirectly so that pg_basebackup doesn't think it's connected to a tty.
	// This ensures that it doesn't print any bare line feeds, which could corrupt other
	// logs.
	// pg_basebackup uses stderr for diagnostic messages and stdout for streaming the backup
	// itself (in some modes; we don't use this). As a result we only need to deal with
	// stderr.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		if _, err := io.Copy(os.Stderr, stderr); err != nil {
			log.Errorf("pg_basebackup failed to copy stderr: %v", err)
		}
	}()

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func (p *Manager) RemoveAll() error {
	initialized, err := p.IsInitialized()
	if err != nil {
		return fmt.Errorf("failed to retrieve instance state: %v", err)
	}
	started := false
	if initialized {
		var err error
		started, err = p.IsStarted()
		if err != nil {
			return fmt.Errorf("failed to retrieve instance state: %v", err)
		}
	}
	if started {
		return fmt.Errorf("cannot remove postregsql database. Instance is active")
	}
	return os.RemoveAll(p.dataDir)
}

func (p *Manager) GetSystemData() (*SystemData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return GetSystemData(ctx, p.replConnParams)
}

func (p *Manager) GetTimelinesHistory(timeline uint64) ([]*TimelineHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return getTimelinesHistory(ctx, timeline, p.replConnParams)
}

func (p *Manager) GetConfigFilePGParameters() (common.Parameters, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return getConfigFilePGParameters(ctx, p.localConnParams)
}

func (p *Manager) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return ping(ctx, p.localConnParams)
}

func (p *Manager) OlderWalFile() (string, error) {
	maj, _, err := p.PGDataVersion()
	if err != nil {
		return "", err
	}
	var walDir string
	if maj < 10 {
		walDir = "pg_xlog"
	} else {
		walDir = "pg_wal"
	}

	f, err := os.Open(filepath.Join(p.dataDir, walDir))
	if err != nil {
		return "", err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return "", err
	}
	sort.Strings(names)

	for _, name := range names {
		if IsWalFileName(name) {
			fi, err := os.Stat(filepath.Join(p.dataDir, walDir, name))
			if err != nil {
				return "", err
			}
			// if the file size is different from the currently supported one
			// (16Mib) return without checking other possible wal files
			if fi.Size() != WalSegSize {
				return "", fmt.Errorf("wal file has unsupported size: %d", fi.Size())
			}
			return name, nil
		}
	}

	return "", nil
}

// IsRestartRequired returns if a postgres restart is necessary
func (p *Manager) IsRestartRequired(changedParams []string) (bool, error) {
	maj, min, err := p.BinaryVersion()
	if err != nil {
		return false, fmt.Errorf("error fetching pg version: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()

	if maj == 9 && min < 5 {
		return isRestartRequiredUsingPgSettingsContext(ctx, p.localConnParams, changedParams)
	} else {
		return isRestartRequiredUsingPendingRestart(ctx, p.localConnParams)
	}
}
