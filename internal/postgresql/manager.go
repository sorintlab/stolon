// Copyright 2026 PgVillage
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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/logging"

	"github.com/mitchellh/copystructure"
)

// Manager manages a PostgreSQL instance
type Manager struct {
	pgBinPath          string
	dataDir            string
	walDir             string
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

// NewManager returns a freshly initialized manager resource
func NewManager(
	pgBinPath string, dataDir,
	walDir string,
	localConnParams,
	replConnParams ConnParams,
	suAuthMethod, suUsername, suPassword, replAuthMethod, replUsername, replPassword string,
	requestTimeout time.Duration,
) *Manager {
	return &Manager{
		pgBinPath:          pgBinPath,
		dataDir:            filepath.Join(dataDir, "postgres"),
		walDir:             walDir,
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

// SetParameters sets PostgreSQL parameters
func (p *Manager) SetParameters(parameters common.Parameters) {
	p.parameters = parameters
}

// CurParameters returns currently set PostgreSQL parameters
func (p *Manager) CurParameters() common.Parameters {
	return p.curParameters
}

// SetRecoveryOptions sets recovery options (when applicable)
func (p *Manager) SetRecoveryOptions(recoveryOptions *RecoveryOptions) {
	if recoveryOptions == nil {
		p.recoveryOptions = NewRecoveryOptions()
		return
	}

	p.recoveryOptions = recoveryOptions
}

// CurRecoveryOptions returns recovery options currently set
func (p *Manager) CurRecoveryOptions() *RecoveryOptions {
	return p.curRecoveryOptions
}

// SetHba sets HBA rules
func (p *Manager) SetHba(hba []string) {
	p.hba = hba
}

// CurHba returns HBA rules currently set
func (p *Manager) CurHba() []string {
	return p.curHba
}

// UpdateCurParameters updates the parameters
func (p *Manager) UpdateCurParameters() {
	var ok bool
	if n, err := copystructure.Copy(p.parameters); err != nil {
		panic(err)
	} else if p.curParameters, ok = n.(common.Parameters); !ok {
		panic("type is different after copy")
	}
}

// UpdateCurRecoveryOptions updates recovery options to new values
func (p *Manager) UpdateCurRecoveryOptions() {
	p.curRecoveryOptions = p.recoveryOptions.DeepCopy()
}

// UpdateCurHba will update HBA rules
func (p *Manager) UpdateCurHba() {
	var ok bool
	if n, err := copystructure.Copy(p.hba); err != nil {
		panic(err)
	} else if p.curHba, ok = n.([]string); !ok {
		panic("type is different after copy")
	}
}

// Init will initialize a PostgreSQL data directory
func (p *Manager) Init(ctx context.Context, initConfig *InitConfig) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	// os.CreateTemp already creates files with urw permissions
	pwfile, err := os.CreateTemp("", "pwfile")
	if err != nil {
		return err
	}
	defer handledFileRemove(pwfile)

	if _, err = pwfile.WriteString(p.suPassword); err != nil {
		return err
	}

	name := filepath.Join(p.pgBinPath, "initdb")
	cmd := exec.Command(name, argDatadir, p.dataDir, "-U", p.suUsername)
	if p.suAuthMethod == "md5" {
		cmd.Args = append(cmd.Args, "--pwfile", pwfile.Name())
	}
	logger.Debug().Str(logCmd, cmd.String()).Msg(logExec)

	// initdb supports configuring a separate wal directory via symlinks. Normally this
	// parameter might be part of the initConfig, but it will also be required whenever we
	// fall-back to a pg_basebackup during a re-sync, which is why it's a Manager field.
	if p.walDir != "" {
		cmd.Args = append(cmd.Args, "--waldir", p.walDir)
	}

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
		if cleanupErr := p.RemoveAll(ctx); cleanupErr != nil {
			logger.Error().AnErr("err", cleanupErr).Msg("failed to cleanup database")
		}
		return err
	}
	return nil
}

// Restore will run a restore command
func (p *Manager) Restore(ctx context.Context, command string) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	var err error
	var cmd *exec.Cmd

	command = expandRecoveryCommand(command, p.dataDir, p.walDir)

	if err = os.MkdirAll(p.dataDir, urwx); err != nil {
		err = fmt.Errorf("cannot create data dir: %v", err)
		goto out
	}
	cmd = exec.Command("/bin/sh", "-c", command)
	logger.Debug().Str(logCmd, cmd.String()).Msg("executing cmd")

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		err = fmt.Errorf("error: %v", err)
		goto out
	}
	// On every error remove the dataDir, so we don't end with an half initialized database

	// TODO: replace goto with defer
out:
	if err != nil {
		if cleanupErr := p.RemoveAll(ctx); cleanupErr != nil {
			logger.Error().AnErr("err", cleanupErr).Msg("failed to cleanup database")
		}
		return err
	}
	return nil
}

// StartTmpMerged starts postgres with a conf file different than
// postgresql.conf, including it at the start of the conf if it exists
func (p *Manager) StartTmpMerged(ctx context.Context) error {
	if err := p.writeConfs(ctx, true); err != nil {
		return err
	}
	tmpPostgresConfPath := filepath.Join(p.dataDir, tmpPostgresConf)

	return p.start(ctx, "-c", fmt.Sprintf("config_file=%s", tmpPostgresConfPath))
}

func (p *Manager) moveWal(ctx context.Context) (err error) {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	var curPath string
	var desiredPath string
	var tmpPath string
	symlinkPath := filepath.Join(p.dataDir, "pg_wal")
	if curPath, err = filepath.EvalSymlinks(symlinkPath); err != nil {
		logger.Error().Str("path", symlinkPath).AnErr("err", err).Msg("could not evaluate symlink")
		return err
	}
	if p.walDir == "" {
		desiredPath = symlinkPath
		tmpPath = filepath.Join(p.dataDir, "pg_wal_new")
	} else {
		desiredPath = p.walDir
		tmpPath = p.walDir
	}
	if curPath == desiredPath {
		return nil
	}
	if p.walDir == "" {
		logger.Info().
			Str("src", curPath).
			Str("temp path", tmpPath).
			Str("dest", desiredPath).
			Msg("moving WAL from src to temp path first and then to dest")
	} else {
		logger.Info().
			Str("src", curPath).
			Str("dest", desiredPath).
			Msg("moving WAL from src to dest")
	}
	// We use tmpPath here first and (if needed) mv tmpPath to desiredPath when all is copied.
	// This allows stolon-keeper to re-read symlink dest and continue should stolon-keeper be restarted while copying.
	if err = moveDirRecursive(ctx, curPath, tmpPath); err != nil {
		return err
	}

	var symlinkStat fs.FileInfo
	if symlinkStat, err = os.Lstat(symlinkPath); errors.Is(err, os.ErrNotExist) {
		logger.Debug().Msg("file or folder already removed")
	} else if err != nil {
		logger.Error().
			Str("path", symlinkPath).
			AnErr("err", err).
			Msg("could not get info on current pg_wal folder/symlink path")
		return err
	} else if symlinkStat.Mode()&os.ModeSymlink != 0 {
		if err = os.Remove(symlinkPath); err != nil {
			logger.Error().
				Str("path", symlinkPath).
				AnErr("err", err).
				Msg("could not remove current pg_wal symlink path")
			return err
		}
	} else if symlinkStat.IsDir() {
		if err = syscall.Rmdir(symlinkPath); err != nil {
			logger.Error().
				Str("path", symlinkPath).
				AnErr("err", err).
				Msg("could not remove current folder")
			return err
		}
	} else {
		err = fmt.Errorf("location %s is no symlink and no dir, so please check and resolve by hand", symlinkPath)
		logger.Error().AnErr("err", err).Msg("")
		return err
	}
	if p.walDir == "" {
		// So we were moving WAL files back into PGDATA. Let's rename the tmpDir now holding all WAL files and use that
		// as PGDATA/pg_wal
		if err = os.Rename(tmpPath, desiredPath); err != nil {
			logger.Error().
				Str("temp path", tmpPath).
				Str("dest", desiredPath).
				AnErr("err", err).
				Msg("cannot move temp path to dest")
			return err
		}
	} else {
		logger.Info().
			Str("src", symlinkPath).
			Str("dest", desiredPath).
			Msg("symlinking src to dest")
		if err = os.Symlink(desiredPath, symlinkPath); err != nil {
			// We were copying WAL files from PGDATA (or another location) to a location outside of PGDATA and
			// pointing the symlink in the right direction failed.
			logger.Error().
				Str("symlink", symlinkPath).
				Str("dest", desiredPath).
				AnErr("err", err).
				Msg("could not create symlink to dest")
			return err
		}
	}
	logger.Info().
		Str("src", curPath).
		Str("dest", desiredPath).
		Msg("moving pg_wal is successful")
	return nil
}

// Start will start the PostgreSQL instance
func (p *Manager) Start(ctx context.Context) error {
	if err := p.writeConfs(ctx, false); err != nil {
		return err
	}
	if err := p.moveWal(ctx); err != nil {
		return err
	}
	return p.start(ctx)
}

// start starts the instance. A success means that the instance has been
// successfully started BUT doesn't mean that the instance is ready to accept
// connections (i.e. it's waiting for some missing wals etc...).
// Note that also on error an instance may still be active and, if needed,
// should be manually stopped calling Stop.
func (p *Manager) start(ctx context.Context, args ...string) error {
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

	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	if err := p.createPostgresqlAutoConf(); err != nil {
		return err
	}

	logger.Info().Msg("starting database")
	name := filepath.Join(p.pgBinPath, "postgres")
	args = append([]string{argDatadir, p.dataDir, "-c",
		"unix_socket_directories=" + common.PgUnixSocketDirectories}, args...)
	cmd := exec.Command(name, args...)
	logger.Debug().Str(logCmd, cmd.String()).Msg(logExec)
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
		fileName := filepath.Join(p.dataDir, "postmaster.pid")
		fh, err := os.Open(fileName)
		defer func() {
			if err := fh.Close(); err != nil {
				logger.Debug().Msgf("failed to close %s: %v", fileName, err)
			}
		}()
		if err == nil {
			scanner := bufio.NewScanner(fh)
			scanner.Split(bufio.ScanLines)
			if scanner.Scan() {
				fpid := scanner.Text()
				if fpid == strconv.Itoa(pid) {
					ok = true
					break
				}
			}
		}

		select {
		case <-exited:
			return errors.New("postgres exited unexpectedly")
		default:
		}

		time.Sleep(200 * time.Millisecond)
	}

	if !ok {
		return errors.New("instance still starting")
	}

	p.UpdateCurParameters()
	p.UpdateCurRecoveryOptions()
	p.UpdateCurHba()

	return nil
}

// Stop tries to stop an instance. An error will be returned if the instance isn't started, stop fails or
// times out (60 second).
func (p *Manager) Stop(ctx context.Context, fast bool) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	logger.Info().Msg("stopping database")
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "stop", "-w", argDatadir, p.dataDir, "-o",
		"-c unix_socket_directories="+common.PgUnixSocketDirectories)
	if fast {
		cmd.Args = append(cmd.Args, "-m", "fast")
	}
	logger.Debug().Str(logCmd, cmd.String()).Msg(logExec)

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: %v", err)
	}
	return nil
}

// IsStarted will return true if PostgreSQL is currently running
func (p *Manager) IsStarted() (bool, error) {
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "status", argDatadir, p.dataDir, "-o",
		"-c unix_socket_directories="+common.PgUnixSocketDirectories)
	_, err := cmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			status := cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
			if status == exitStatusNotRunning {
				return false, nil
			}
			if status == exitStatusInaccessibleDatadir {
				return false, ErrInaccessibleDatadir
			}
		}
		return false, fmt.Errorf("cannot get instance state: %v", err)
	}
	return true, nil
}

// Reload will trigger a reload in PostgreSQL
func (p *Manager) Reload(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	logger.Info().Msg("reloading database configuration")

	if err := p.writeConfs(ctx, false); err != nil {
		return err
	}

	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "reload", argDatadir, p.dataDir, "-o",
		"-c unix_socket_directories="+common.PgUnixSocketDirectories)
	logger.Debug().Str(logCmd, cmd.String()).Msg(logExec)

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
func (p *Manager) StopIfStarted(ctx context.Context, fast bool) error {
	// Stop will return an error if the instance isn't started, so first check
	// if it's started
	started, err := p.IsStarted()
	if err != nil {
		if err == ErrInaccessibleDatadir {
			// if IsStarted returns an unknown state error then assume that the
			// instance is stopped
			return nil
		}
		return err
	}
	if !started {
		return nil
	}
	if err = p.Stop(ctx, fast); err != nil {
		return err
	}
	started, err = p.IsStarted()
	if err != nil {
		return err
	}
	if started {
		return errors.New("failed to stop")
	}
	return nil
}

// Restart will stop (if started) and start PostgreSQL
func (p *Manager) Restart(ctx context.Context, fast bool) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	logger.Info().Msg("restarting database")
	if err := p.StopIfStarted(ctx, fast); err != nil {
		return err
	}
	return p.Start(ctx)
}

// WaitReady will wait for PostgreSQL to be available
func (p *Manager) WaitReady(ctx context.Context, timeout time.Duration) error {
	start := time.Now()
	for timeout == 0 || time.Since(start) < timeout {
		if err := p.Ping(ctx); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("timeout waiting for db ready")
}

// WaitRecoveryDone will wait for recovery to be done (signal or done file)
func (p *Manager) WaitRecoveryDone(timeout time.Duration) error {
	version, err := p.BinaryVersion()
	if err != nil {
		return fmt.Errorf("error fetching pg version: %v", err)
	}

	start := time.Now()
	if version.GreaterThanEqual(V12) {
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

	return errors.New("timeout waiting for db recovery")
}

// PGDataVersion returns the version of the PostgreSQL data directory
func (p *Manager) PGDataVersion() (*semver.Version, error) {
	return pgDataVersion(p.dataDir)
}

// BinaryVersion returns the version of the PostgreSQL binaries
func (p *Manager) BinaryVersion() (*semver.Version, error) {
	return binaryVersion(p.pgBinPath)
}

// Promote will promote PostgreSQL (with pg_ctl)
func (p *Manager) Promote(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	logger.Info().Msg("promoting database")

	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "promote", "-w", argDatadir, p.dataDir)
	logger.Debug().Str(logCmd, cmd.String()).Msg("executing cmd")

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: %v", err)
	}

	return p.writeConfs(ctx, false)
}

// SetupRoles will connect to PostgreSQL to setup all users as required by stolon
func (p *Manager) SetupRoles(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()

	if p.suUsername == p.replUsername {
		logger.Info().Msg("adding replication role to superuser")
		if p.suAuthMethod == "trust" {
			if err := alterPasswordlessRole(ctx, p.localConnParams, []string{"replication"}, p.suUsername); err != nil {
				return fmt.Errorf("error adding replication role to superuser: %v", err)
			}
		} else {
			if err := alterRole(
				ctx,
				p.localConnParams,
				[]string{"replication"},
				p.suUsername,
				p.suPassword,
			); err != nil {
				return fmt.Errorf("error adding replication role to superuser: %v", err)
			}
		}
		logger.Info().Msg("replication role added to superuser")
	} else {
		// Configure superuser role password if auth method is not trust
		if p.suAuthMethod != "trust" && p.suPassword != "" {
			logger.Info().Msg("setting superuser password")
			if err := setPassword(ctx, p.localConnParams, p.suUsername, p.suPassword); err != nil {
				return fmt.Errorf("error setting superuser password: %v", err)
			}
			logger.Info().Msg("superuser password set")
		}
		roles := []string{"login", "replication"}
		logger.Info().Msg("creating replication role")
		if p.replAuthMethod != "trust" {
			if err := createRole(ctx, p.localConnParams, roles, p.replUsername, p.replPassword); err != nil {
				return fmt.Errorf("error creating replication role: %v", err)
			}
		} else {
			if err := createPasswordlessRole(ctx, p.localConnParams, roles, p.replUsername); err != nil {
				return fmt.Errorf("error creating replication role: %v", err)
			}
		}
		logger.Info().Str("role", p.replUsername).Msg("replication role created")
	}
	return nil
}

// GetSyncStandbys returns the standby's (from pg_stat_replication)
func (p *Manager) GetSyncStandbys(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return getSyncStandbys(ctx, p.localConnParams)
}

// GetReplicationSlots (from pg_replication_slots)
func (p *Manager) GetReplicationSlots(ctx context.Context) ([]string, error) {
	version, err := p.PGDataVersion()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return getReplicationSlots(ctx, p.localConnParams, version)
}

// CreateReplicationSlot will create a replication slot
func (p *Manager) CreateReplicationSlot(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return createReplicationSlot(ctx, p.localConnParams, name)
}

// DropReplicationSlot will drop a replication slot
func (p *Manager) DropReplicationSlot(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return dropReplicationSlot(ctx, p.localConnParams, name)
}

// IsInitialized checks if a datadirectory is already initialized
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
	version, err := p.PGDataVersion()
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
	if version.LessThan(V10) {
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
	version, err := p.BinaryVersion()
	if err != nil {
		return "", fmt.Errorf("error fetching pg version: %v", err)
	}

	if version.GreaterThanEqual(V12) {
		// if standby.signal file exists then consider it as a standby
		_, err := os.Stat(filepath.Join(p.dataDir, postgresStandbySignal))
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("error determining if %q file exists: %v", postgresStandbySignal, err)
		}
		if os.IsNotExist(err) {
			return common.RolePrimary, nil
		}
		return common.RoleReplica, nil
	}
	// if recovery.conf file exists then consider it as a standby
	_, err = os.Stat(filepath.Join(p.dataDir, postgresRecoveryConf))
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("error determining if %q file exists: %v", postgresRecoveryConf, err)
	}
	if os.IsNotExist(err) {
		return common.RolePrimary, nil
	}
	return common.RoleReplica, nil
}

func (p *Manager) writeConfs(ctx context.Context, useTmpPostgresConf bool) error {
	version, err := p.BinaryVersion()
	if err != nil {
		return fmt.Errorf("error fetching pg version: %v", err)
	}

	writeRecoveryParamsInPostgresConf := false
	if version.GreaterThanEqual(V12) {
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
		if err := p.writeStandbySignal(ctx); err != nil {
			return fmt.Errorf("error writing %s file: %v", postgresStandbySignal, err)
		}
		if err := p.writeRecoverySignal(ctx); err != nil {
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

	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, confFile), urw,
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
				ev := strings.Replace(v, "'", "''", -1)
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

	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, postgresRecoveryConf), urw,
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

func (p *Manager) writeStandbySignal(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	// write standby.signal only if recoveryMode is standby
	if p.recoveryOptions.RecoveryMode != RecoveryModeStandby {
		return nil
	}

	logger.Info().Msg("writing standby signal file")

	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, postgresStandbySignal), urw,
		func(_ io.Writer) error {
			return nil
		})
}

func (p *Manager) writeRecoverySignal(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	// write standby.signal only if recoveryMode is recovery
	if p.recoveryOptions.RecoveryMode != RecoveryModeRecovery {
		return nil
	}

	logger.Info().Msg("writing recovery signal file")

	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, postgresRecoverySignal), urw,
		func(_ io.Writer) error {
			return nil
		})
}

func (p *Manager) writePgHba() error {
	return common.WriteFileAtomicFunc(filepath.Join(p.dataDir, "pg_hba.conf"), urw,
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

// SyncFromFollowedPGRewind runs pgrewind to get to a shared point in the WAL stream
func (p *Manager) SyncFromFollowedPGRewind(ctx context.Context, followedConnParams ConnParams, password string) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	// Remove postgresql.auto.conf since pg_rewind will error if it's a symlink to /dev/null
	pgAutoConfPath := filepath.Join(p.dataDir, postgresAutoConf)
	if err := os.Remove(pgAutoConfPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing postgresql.auto.conf file: %v", err)
	}

	// os.CreateTemp already creates files with urw permissions
	pgpass, err := os.CreateTemp("", "pgpass")
	if err != nil {
		return err
	}
	defer handledFileRemove(pgpass)

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

	logger.Info().Msg("running pg_rewind")
	name := filepath.Join(p.pgBinPath, "pg_rewind")
	cmd := exec.Command(name, "--debug", argDatadir, p.dataDir, "--source-server="+followedConnString)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSFILE=%s", pgpass.Name()))
	logger.Debug().Str(logCmd, cmd.String()).Msg(logExec)

	// Pipe command's std[err|out] to parent.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: %v", err)
	}
	return nil
}

// SyncFromFollowed runs pg_basebackup to resync a broken replica
func (p *Manager) SyncFromFollowed(ctx context.Context, followedConnParams ConnParams, replSlot string) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	fcp := followedConnParams.Copy()

	//  already creates files with urw permissions
	pgpass, err := os.CreateTemp("", "pgpass")
	if err != nil {
		return err
	}
	defer handledFileRemove(pgpass)

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

	logger.Info().Msg("running pg_basebackup")
	name := filepath.Join(p.pgBinPath, "pg_basebackup")
	args := []string{"-R", "-v", "-P", "-Xs", argDatadir, p.dataDir, argDbName, followedConnString}
	if replSlot != "" {
		args = append(args, "--slot", replSlot)
	}
	if p.walDir != "" {
		args = append(args, "--waldir", p.walDir)
	}
	cmd := exec.Command(name, args...)

	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSFILE=%s", pgpass.Name()))
	logger.Debug().Str(logCmd, cmd.String()).Msg(logExec)

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
			logger.Error().AnErr("err", err).Msg("pg_basebackup failed to copy stderr")
		}
	}()

	return cmd.Wait()
}

// RemoveAllIfInitialized is a safe way to clean a datadirectory before recreating
func (p *Manager) RemoveAllIfInitialized(ctx context.Context) error {
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
		return errors.New("cannot remove postregsql database. Instance is active")
	}

	return p.RemoveAll(ctx)
}

// RemoveAll entirely cleans up the data directory, including any wal directory if that
// exists outside of the data directory.
func (p *Manager) RemoveAll(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	if p.walDir != "" {
		if err := os.RemoveAll(p.walDir); err != nil {
			logger.Fatal().Str("path", p.walDir).AnErr("err", err).Msg("failed to remove tree")
		}
	}

	return os.RemoveAll(p.dataDir)
}

// GetSystemData will retrieve and return the system data (IDENTIFY_SYSTEM)
func (p *Manager) GetSystemData(ctx context.Context) (*SystemData, error) {
	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return GetSystemData(ctx, p.replConnParams)
}

// GetTimelinesHistory will return a lost of timelinehostory objects
func (p *Manager) GetTimelinesHistory(ctx context.Context, timeline uint64) ([]*TimelineHistory, error) {
	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return getTimelinesHistory(ctx, timeline, p.replConnParams)
}

// GetConfigFilePGParameters will return the config file parameters (pg_file_settings or pg_settings)
func (p *Manager) GetConfigFilePGParameters(ctx context.Context) (common.Parameters, error) {
	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return getConfigFilePGParameters(ctx, p.localConnParams)
}

// Ping triesd to connect to PostgreSQL and returns an error if it can't
func (p *Manager) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return ping(ctx, p.localConnParams)
}

// OlderWalFile returns the oldest WAL file in the pg_wal location
func (p *Manager) OlderWalFile() (string, error) {
	version, err := p.PGDataVersion()
	if err != nil {
		return "", err
	}
	var walDir string
	if version.LessThan(V10) {
		walDir = "pg_xlog"
	} else {
		walDir = "pg_wal"
	}

	f, err := os.Open(filepath.Join(p.dataDir, walDir))
	if err != nil {
		return "", err
	}
	names, err := f.Readdirnames(-1)
	handledFileClose(f)
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
			if fi.Size() != walSegSize {
				return "", fmt.Errorf("wal file has unsupported size: %d", fi.Size())
			}
			return name, nil
		}
	}

	return "", nil
}

// IsRestartRequired returns if a postgres restart is necessary
func (p *Manager) IsRestartRequired(ctx context.Context, changedParams []string) (bool, error) {
	version, err := p.BinaryVersion()
	if err != nil {
		return false, fmt.Errorf("error fetching pg version: %v", err)
	}

	ctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()

	if version.LessThan(V95) {
		return isRestartRequiredUsingPgSettingsContext(ctx, p.localConnParams, changedParams)
	}
	return isRestartRequiredUsingPendingRestart(ctx, p.localConnParams)
}

// GetSystemID is function that fetches the systemID and returns it as a string
func (p *Manager) GetSystemID() (string, error) {
	pgControlFile := filepath.Join(p.dataDir, "global", "pg_control")
	pgControl, err := os.Open(pgControlFile)
	if err != nil {
		return "", err
	}
	defer handledFileClose(pgControl)
	var systemID uint64
	err = binary.Read(pgControl, binary.NativeEndian, &systemID)
	if err != nil {
		return "", err
	}
	const baseTen = 10
	return strconv.FormatUint(systemID, baseTen), nil
}
