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
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"

	"github.com/sorintlab/stolon/common"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/pkg/capnslog"
	_ "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/lib/pq"
)

var (
	log = capnslog.NewPackageLogger("github.com/sorintlab/stolon/pkg", "postgresql")
)

type Manager struct {
	name           string
	dataDir        string
	listenAddress  string
	port           string
	replUser       string
	replPassword   string
	connString     string
	pgBinPath      string
	requestTimeout time.Duration
}

var DefaultParameters = map[string]string{
	"unix_socket_directories": "/tmp",
	"archive_mode":            "on",
	"wal_level":               "hot_standby",
	"archive_command":         "mkdir -p ../wal_archive && cp %p ../wal_archive/%f",
	"max_wal_senders":         "5",
	"wal_keep_segments":       "8",
	"archive_timeout":         "1800s",
	"max_replication_slots":   "5",
	"hot_standby":             "on",
}

func NewManager(name string, pgBinPath string, dataDir string, listenAddress, port, replUser, replPassword string, requestTimeout time.Duration) (*Manager, error) {
	connString := fmt.Sprintf("postgres://%s:%s/postgres?sslmode=disable", "127.0.0.1", port)
	return &Manager{
		name:           name,
		dataDir:        filepath.Join(dataDir, "postgres"),
		listenAddress:  listenAddress,
		port:           port,
		replUser:       replUser,
		replPassword:   replPassword,
		connString:     connString,
		pgBinPath:      pgBinPath,
		requestTimeout: requestTimeout,
	}, nil
}

func (p *Manager) Init() error {
	name := filepath.Join(p.pgBinPath, "initdb")
	out, err := exec.Command(name, "-D", p.dataDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, out)
	}
	err = p.Start()
	if err != nil {
		return err
	}
	log.Infof("Setting required accesses to pg_hba.conf\n")
	err = p.writePgHba()
	if err != nil {
		return fmt.Errorf("error setting requires accesses to pg_hba.conf: %v", err)
	}
	err = p.Reload()
	if err != nil {
		return err
	}

	log.Infof("Creating repl user\n")
	err = p.CreateReplUser()
	if err != nil {
		return fmt.Errorf("error creating replication user: %v", err)
	}
	err = p.Stop(true)
	if err != nil {
		return err
	}
	return nil
}

func (p *Manager) Start() error {
	log.Infof("Starting database\n")
	serverArgs := p.getServerArguments()
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "start", "-w", "-D", p.dataDir, "-o", serverArgs)
	var out bytes.Buffer
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, out)
	}

	log.Infof("pid: %d\n", cmd.Process.Pid)

	err = cmd.Wait()

	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, out)
	}
	return nil
}

func (p *Manager) Stop(fast bool) error {
	log.Infof("Stopping database\n")
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "stop", "-w", "-D", p.dataDir, "-o", "-c unix_socket_directories=/tmp")
	if fast {
		cmd.Args = append(cmd.Args, "-m", "fast")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, out)
	}
	return nil
}

func (p *Manager) IsStarted() (bool, error) {
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "status", "-w", "-D", p.dataDir, "-o", "-c unix_socket_directories=/tmp")
	_, err := cmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			status := cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
			if status == 3 {
				return false, nil
			}
		}
		return false, fmt.Errorf("cannot get instance state: %v", err)
	}
	return true, nil
}

func (p *Manager) Reload() error {
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "reload", "-D", p.dataDir, "-o", "-c unix_socket_directories=/tmp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, out)
	}
	return nil
}

func (p *Manager) Restart(fast bool) error {
	log.Infof("Restarting database\n")
	err := p.Stop(true)
	if err != nil {
		return err
	}
	err = p.Start()
	if err != nil {
		return err
	}
	return nil
}

func (p *Manager) Promote() error {
	log.Infof("Promoting database\n")
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "promote", "-w", "-D", p.dataDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, out)
	}
	return nil
}

func (p *Manager) BecomeSlave(masterconnString string) error {
	err := p.WriteRecoveryConf(masterconnString)
	if err != nil {
		return err
	}
	return nil
}

func (p *Manager) CreateReplUser() error {
	db, err := sql.Open("postgres", p.connString)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	_, err = Exec(ctx, db, fmt.Sprintf(`CREATE USER "%s" WITH REPLICATION ENCRYPTED PASSWORD '%s';`, p.replUser, p.replPassword))
	cancel()
	return err
}

func (p *Manager) GetReplicatinSlots() ([]string, error) {
	db, err := sql.Open("postgres", p.connString)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	replSlots := []string{}

	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	rows, err := Query(ctx, db, "SELECT slot_name from pg_replication_slots")
	cancel()
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

func (p *Manager) CreateReplicationSlot(name string) error {
	db, err := sql.Open("postgres", p.connString)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	_, err = Exec(ctx, db, fmt.Sprintf("select pg_create_physical_replication_slot('%s')", name))
	cancel()
	return err
}

func (p *Manager) DropReplicationSlot(name string) error {
	db, err := sql.Open("postgres", p.connString)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	_, err = Exec(ctx, db, fmt.Sprintf("select pg_drop_replication_slot('%s')", name))
	cancel()
	return err
}

func (p *Manager) IsInitialized() (bool, error) {
	// TODO improve checks
	dir, err := os.Open(p.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	n, err := dir.Readdirnames(1)
	if err != nil && err != io.EOF {
		return false, err
	}
	if len(n) > 0 {
		return true, nil
	}
	return false, nil
}

func (p *Manager) getServerArguments() string {
	var args string
	args += fmt.Sprintf("-c listen_addresses=127.0.0.1,%s -c port=%s", p.listenAddress, p.port)
	for k, v := range DefaultParameters {
		args += fmt.Sprintf(" -c \"%s=%s\"", k, v)
	}
	log.Debugf("server arguments: %s", args)
	return args
}

func (p *Manager) GetRoleFromDB() (common.Role, error) {
	db, err := sql.Open("postgres", p.connString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	rows, err := Query(ctx, db, "SELECT pg_is_in_recovery from pg_is_in_recovery()")
	cancel()
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var isInRecovery bool
		if err := rows.Scan(&isInRecovery); err != nil {
			return 0, err
		}
		if isInRecovery {
			return common.SlaveRole, nil
		}
		return common.MasterRole, nil
	}
	return 0, fmt.Errorf("cannot get pg role from db: no rows returned")
}

func (p *Manager) GetRole() (common.Role, error) {
	curConnParams, err := p.GetPrimaryConninfo()
	if err != nil {
		return 0, fmt.Errorf("error retrieving primary conn info: %v", err)
	}
	if curConnParams == nil {
		return common.MasterRole, nil
	}
	return common.SlaveRole, nil
}

func (p *Manager) GetPrimaryConninfo() (connParams, error) {
	regex, err := regexp.Compile(`\s*primary_conninfo\s*=\s*'(.*)'$`)
	if err != nil {
		return nil, err
	}
	fh, err := os.Open(filepath.Join(p.dataDir, "recovery.conf"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		m := regex.FindStringSubmatch(scanner.Text())
		if len(m) == 2 {
			connString := m[1]
			return ParseConnString(connString)
		}
	}
	return nil, nil
}

func (p *Manager) HasconnString() (bool, error) {
	regex, err := regexp.Compile(`primary_conninfo`)
	if err != nil {
		return false, err
	}
	fh, err := os.Open(filepath.Join(p.dataDir, "recovery.conf"))
	if os.IsNotExist(err) {
		return false, nil
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		if regex.MatchString(scanner.Text()) {
			return true, nil
		}
	}
	return false, nil
}

func (p *Manager) WriteRecoveryConf(masterconnString string) error {
	f, err := os.Create(filepath.Join(p.dataDir, "recovery.conf"))
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("standby_mode = 'on'\n")
	f.WriteString(fmt.Sprintf("primary_slot_name = '%s'\n", p.name))
	f.WriteString("recovery_target_timeline = 'latest'\n")

	if masterconnString != "" {
		cp, err := URLToConnParams(masterconnString)
		if err != nil {
			return err
		}
		f.WriteString(fmt.Sprintf("primary_conninfo = '%s'", cp.ConnString()))
	}
	return nil
}

func (p *Manager) writePgHba() error {
	f, err := os.OpenFile(filepath.Join(p.dataDir, "pg_hba.conf"), os.O_APPEND|os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// TODO(sgotti) Do not set this but let the user provide its ph_hba.conf file/entries
	f.WriteString(fmt.Sprintf("host all %s %s md5\n", p.replUser, "0.0.0.0/0"))
	f.WriteString(fmt.Sprintf("host all %s %s md5\n", p.replUser, "::0/0"))
	// TODO(sgotti) Configure this dynamically based on our followers provided by the clusterview
	f.WriteString(fmt.Sprintf("host replication %s %s md5\n", p.replUser, "0.0.0.0/0"))
	f.WriteString(fmt.Sprintf("host replication %s %s md5\n", p.replUser, "::0/0"))
	return nil
}

func (p *Manager) SyncFromMaster(masterconnString string) error {
	cp, err := URLToConnParams(masterconnString)
	if err != nil {
		return err
	}

	pgpass, err := ioutil.TempFile("", "pgpass")
	if err != nil {
		return err
	}
	defer os.Remove(pgpass.Name())
	defer pgpass.Close()

	pgpass.WriteString(fmt.Sprintf("%s:%s:*:%s:%s\n", cp.Get("host"), cp.Get("port"), cp.Get("user"), cp.Get("password")))

	log.Infof("Running pg_basebackup\n")
	name := filepath.Join(p.pgBinPath, "pg_basebackup")
	cmd := exec.Command(name, "-R", "-D", p.dataDir, "--host="+cp.Get("host"), "--port="+cp.Get("port"), "-U", cp.Get("user"))
	cmd.Env = append(cmd.Env, fmt.Sprintf("PGPASSFILE=%s", pgpass.Name()))
	log.Debugf("execing cmd: %s", cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, out)
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
