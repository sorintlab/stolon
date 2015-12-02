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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sorintlab/stolon/common"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/pkg/capnslog"
	_ "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/lib/pq"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
)

var (
	log = capnslog.NewPackageLogger("github.com/sorintlab/stolon/pkg", "postgresql")
)

type Manager struct {
	name           string
	dataDir        string
	replUser       string
	replPassword   string
	connString     string
	replConnString string
	pgBinPath      string
	requestTimeout time.Duration
	parameters     Parameters
	confDir        string
}

type Parameters map[string]string

func (s Parameters) Copy() Parameters {
	parameters := Parameters{}
	for k, v := range s {
		parameters[k] = v
	}
	return parameters
}

func (s Parameters) Set(k, v string) {
	s[k] = v
}

func (s Parameters) Get(k string) (string, bool) {
	v, ok := s[k]
	return v, ok
}

func (s Parameters) Equals(is Parameters) bool {
	return reflect.DeepEqual(s, is)
}

func NewManager(name string, pgBinPath string, dataDir string, confDir string, parameters Parameters, connString, replConnString, replUser, replPassword string, requestTimeout time.Duration) (*Manager, error) {
	return &Manager{
		name:           name,
		dataDir:        filepath.Join(dataDir, "postgres"),
		replUser:       replUser,
		replPassword:   replPassword,
		connString:     connString,
		replConnString: replConnString,
		pgBinPath:      pgBinPath,
		requestTimeout: requestTimeout,
		parameters:     parameters,
		confDir:        confDir,
	}, nil
}

func (p *Manager) SetParameters(parameters Parameters) {
	p.parameters = parameters
}

func (p *Manager) GetParameters() Parameters {
	return p.parameters
}

func (p *Manager) Init() error {
	name := filepath.Join(p.pgBinPath, "initdb")
	out, err := exec.Command(name, "-D", p.dataDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, out)
	}
	// Move current (initdb generated) postgresql.conf ot postgresql-base.conf
	if err := os.Rename(filepath.Join(p.dataDir, "postgresql.conf"), filepath.Join(p.dataDir, "postgresql-base.conf")); err != nil {
		return fmt.Errorf("error moving postgresql.conf file to postgresql-base.conf: %v", err)
	}
	// Create default confDir
	if err := os.Mkdir(filepath.Join(p.dataDir, "conf.d"), 0700); err != nil {
		return fmt.Errorf("error creating conf.d inside dataDir: %v", err)
	}
	if err := p.WriteConf(); err != nil {
		return fmt.Errorf("error writing postgresql.conf file: %v", err)
	}

	log.Infof("Setting required accesses to pg_hba.conf")
	if err := p.writePgHba(); err != nil {
		return fmt.Errorf("error setting requires accesses to pg_hba.conf: %v", err)
	}

	if err := p.Start(); err != nil {
		return fmt.Errorf("error starting instance: %v", err)
	}
	log.Infof("Creating replication role")
	if err := p.CreateReplRole(); err != nil {
		return fmt.Errorf("error creating replication role: %v", err)
	}
	err = p.Stop(true)
	if err != nil {
		return err
	}
	return nil
}

func (p *Manager) Start() error {
	log.Infof("Starting database")
	if err := p.WriteConf(); err != nil {
		return fmt.Errorf("error writing conf file: %v", err)
	}
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "start", "-w", "-D", p.dataDir)
	// TODO(sgotti) attaching a pipe to sdtout/stderr makes the postgres
	// process executed by pg_ctl inheriting it's file descriptors. So
	// cmd.Wait() will block and waiting on them to be closed (will happend
	// only when postgres is stopped). So this functions will never return.
	// To avoid this no output is captured. If needed there's the need to
	// find a way to get the output whitout blocking.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: %v", err)
	}
	return nil
}

func (p *Manager) Stop(fast bool) error {
	log.Infof("Stopping database")
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "stop", "-w", "-D", p.dataDir, "-o", "-c unix_socket_directories=/tmp")
	if fast {
		cmd.Args = append(cmd.Args, "-m", "fast")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: %v, output: %s", err, string(out))
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
	log.Infof("Reloading database configuration")
	if err := p.WriteConf(); err != nil {
		return fmt.Errorf("error writing conf file: %v", err)
	}
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "reload", "-D", p.dataDir, "-o", "-c unix_socket_directories=/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error: %v, output: %s", err, string(out))
	}
	return nil
}

func (p *Manager) Restart(fast bool) error {
	log.Infof("Restarting database")
	if err := p.Stop(true); err != nil {
		return err
	}
	if err := p.Start(); err != nil {
		return err
	}
	return nil
}

func (p *Manager) Promote() error {
	log.Infof("Promoting database")
	name := filepath.Join(p.pgBinPath, "pg_ctl")
	cmd := exec.Command(name, "promote", "-w", "-D", p.dataDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error: %v, output: %s", err, string(out))
	}
	return nil
}

func (p *Manager) BecomeStandby(masterconnString string) error {
	return p.WriteRecoveryConf(masterconnString)
}

func (p *Manager) CreateReplRole() error {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return CreateReplRole(ctx, p.connString, p.replUser, p.replPassword)
}

func (p *Manager) GetReplicatinSlots() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return GetReplicatinSlots(ctx, p.connString)
}

func (p *Manager) CreateReplicationSlot(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return CreateReplicationSlot(ctx, p.connString, name)
}

func (p *Manager) DropReplicationSlot(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return DropReplicationSlot(ctx, p.connString, name)
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

func (p *Manager) GetRoleFromDB() (common.Role, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
	defer cancel()
	return GetRole(ctx, p.connString)
}

func (p *Manager) GetRole() (common.Role, error) {
	curConnParams, err := p.GetPrimaryConninfo()
	if err != nil {
		return 0, fmt.Errorf("error retrieving primary conn info: %v", err)
	}
	if curConnParams == nil {
		return common.MasterRole, nil
	}
	return common.StandbyRole, nil
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
			return ParseConnString(m[1])
		}
	}
	return nil, nil
}

func (p *Manager) HasConnString() (bool, error) {
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

func (p *Manager) WriteConf() error {
	f, err := ioutil.TempFile(p.dataDir, "postgresql.conf")
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("include 'postgresql-base.conf'\n")
	if p.confDir != "" {
		f.WriteString(fmt.Sprintf("include_dir '%s'\n", p.confDir))
	} else {
		f.WriteString("include_dir 'conf.d'\n")
	}
	for k, v := range p.parameters {
		// Single quotes needs to be doubled
		ev := strings.Replace(v, `'`, `''`, -1)
		_, err = f.WriteString(fmt.Sprintf("%s = '%s'\n", k, ev))
		if err != nil {
			os.Remove(f.Name())
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(p.dataDir, "postgresql.conf")); err != nil {
		os.Remove(f.Name())
		return err
	}

	return nil
}

func (p *Manager) WriteRecoveryConf(masterconnString string) error {
	f, err := ioutil.TempFile(p.dataDir, "recovery.conf")
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("standby_mode = 'on'\n")
	f.WriteString(fmt.Sprintf("primary_slot_name = '%s'\n", p.name))
	f.WriteString("recovery_target_timeline = 'latest'\n")

	if masterconnString != "" {
		var cp connParams
		cp, err = URLToConnParams(masterconnString)
		if err != nil {
			return err
		}
		f.WriteString(fmt.Sprintf("primary_conninfo = '%s'", cp.ConnString()))
	}
	if err := f.Sync(); err != nil {
		return err
	}

	if err = os.Rename(f.Name(), filepath.Join(p.dataDir, "recovery.conf")); err != nil {
		os.Remove(f.Name())
		return err
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
	f.WriteString("host all all 0.0.0.0/0 md5\n")
	f.WriteString("host all all ::0/0 md5\n")
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
	// First, we try to do pg_rewind.
	if PGRewindCanBeUsed(p.dataDir) == true {
		log.Infof("Running pg_rewind")
		if err = p.SyncFromMasterByPGRewind(masterconnString); err == nil {
			return nil // pg_rewind successfully finished.
		}
	}
	// Well, we do pg_basebackup.
	pgpass, err := ioutil.TempFile("", "pgpass")
	if err != nil {
		return err
	}
	defer os.Remove(pgpass.Name())
	defer pgpass.Close()

	err = p.RemoveAll()
	if err != nil {
		return fmt.Errorf("failed to remove the postgres data dir: %v", err)
	}
	host := cp.Get("host")
	port := cp.Get("port")
	user := cp.Get("user")
	password := cp.Get("password")
	pgpass.WriteString(fmt.Sprintf("%s:%s:*:%s:%s\n", host, port, user, password))

	log.Infof("Running pg_basebackup")
	name := filepath.Join(p.pgBinPath, "pg_basebackup")
	cmd := exec.Command(name, "-R", "-D", p.dataDir, "--host="+host, "--port="+port, "-U", user)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PGPASSFILE=%s", pgpass.Name()))
	log.Debugf("execing cmd: %s", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error: %v, output: %s", err, string(out))
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

func getConfParams(basedir, file string, params map[string]string) error {
	confFile := basedir + "/" + file

	fp, err := os.Open(confFile)
	if err != nil {
		return err
	}
	defer fp.Close()

	rep_inc, _ := regexp.Compile("^include\\s+")
	rep_incdir, _ := regexp.Compile("^include_dir\\s+")

	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		s := scanner.Text()
		if rep_inc.MatchString(s) {
			s = regexp.MustCompile("include").ReplaceAllString(s, "")
			s = strings.Trim(strings.TrimRight(strings.TrimSpace(s), "\n"), "'")
			if err = getConfParams(basedir, s, params); err != nil {
				return err
			}
		} else if rep_incdir.MatchString(s) {
			s = regexp.MustCompile("include_dir").ReplaceAllString(s, "")
			s = strings.Trim(strings.TrimRight(strings.TrimSpace(s), "\n"), "'")
			files, _ := ioutil.ReadDir(basedir + "/" + s)
			for _, f := range files {
				if err = getConfParams(basedir+"/"+s, f.Name(), params); err != nil {
					return err
				}
			}
		} else {
			pf1 := "^\\s*[#]*\\s*[\\w\\/\\_\\-]+\\s*=\\s*\\'[\\w\\/\\.\\,\\_\\-\\%\\s*\\$\\:\\!\\+\"]*\\'"
			pf2 := "^\\s*[#]*\\s*[\\w\\/\\_\\-]+\\s*=\\s*[\\w\\/\\.\\,\\_\\-]+"
			pformats := []string{pf1, pf2}
			for _, v := range pformats {
				if match, _ := regexp.MatchString(v, s); match == true {
					// Delete ^#
					if match, _ = regexp.MatchString("^\\s*#", s); match == true {
						s = strings.TrimLeft(s, "#")
					}
					line := strings.Split(s, "#")     // Delete comment
					pv := strings.Split(line[0], "=") // Get param and value
					pv[0] = strings.TrimSpace(pv[0])
					pv[1] = strings.TrimSpace(pv[1])
					params[pv[0]] = pv[1]
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	exceptions := []string{"TB", "GB", "MB", "name", "d"} // These are samples in comment.
	for _, v := range exceptions {
		if _, ex := params[v]; ex == true {
			delete(params, v)
		}
	}
	return nil
}

func PGRewindCanBeUsed(dataDir string) bool {
	var version string

	// Check cluster version; it should be greater than or equal to 9.5
	if _, err := os.Stat(dataDir + "/PG_VERSION"); err != nil {
		log.Debugf("the specified base directory:%s not found.\n", dataDir)
		return false
	}
	if rawBytes, err := ioutil.ReadFile(dataDir + "/PG_VERSION"); err != nil {
		log.Debugf("PG_VERSION file not read.\n")
		return false
	} else {
		log.Debugf("database cluster version is %s\n", version)
		version = strings.TrimRight(string(rawBytes), "\n")
	}
	vformat := "^\\d+\\.\\d+\\.*\\d*$"
	if _, err := regexp.MatchString(vformat, version); err != nil {
		log.Debugf("The value of PG_VERSION is invalid.\n")
		return false
	}
	lowestVersion := [3]int{9, 5, 0}
	for k, x := range strings.Split(version, ".") {
		v, _ := strconv.Atoi(x)
		if v < lowestVersion[k] {
			log.Debugf("pg_rewind cannot be used in version %s\n", version)
			return false
		}
	}
	// Check recovery.conf
	if _, err := os.Stat(dataDir + "/recovery.conf"); err != nil {
		log.Debugf("There is no recovery.conf.\n")
		return false
	}
	// Check parameter values
	params := map[string]string{}
	if err := getConfParams(dataDir, "postgresql.conf", params); err != nil {
		log.Debugf("ERRER: %v.\n", err)
		return false
	}
	for _, x := range []string{"wal_log_hints", "full_page_writes"} {
		if strings.ToLower(strings.Trim(params[x], "'")) != "on" {
			log.Debugf("%s is disabled.\n", x)
			return false
		}
	}
	log.Debugf("pg_rewind can be used.\n")
	return true
}

func makeConnStr(masterconnString string) (string, error) {
	connstr := ""
	cp, err := URLToConnParams(masterconnString)
	if err != nil {
		return connstr, err
	}
	var m map[string]string = make(map[string]string)
	m["host"] = cp.Get("host")
	m["port"] = cp.Get("port")

	// TODO: superuser authentication
	m["user"] = "postgres"
	//m["password"] = cp.Get("password")

	for k, v := range m {
		if v != "" {
			connstr += k + "=" + v + " "
		}
	}
	connstr = strings.TrimRight(connstr, " ")
	log.Debugf("pg_rewind's connstr:%s", connstr)

	return connstr, nil
}

func (p *Manager) SyncFromMasterByPGRewind(masterconnString string) error {
	// Change the database cluster's state from "shut down in recovery" to "shut down"
	//
	// Though pg_rewind requires that the cluster's state is "shut down",
	// the stopped standby's state is "shut down in recovery".
	// Therefore, we should start the database instance in normal mode,
	// and then we should stop it again.
	if err := os.Remove(p.dataDir + "/recovery.conf"); err != nil {
		return err
	}
	if err := p.Start(); err != nil {
		return err
	}
	if err := p.Stop(true); err != nil {
		return err
	}

	// Do pg_rewind
	name := filepath.Join(p.pgBinPath, "pg_rewind")
	if connstr, err := makeConnStr(masterconnString); err != nil {
		log.Errorf("error stopping instance: %v", err)
		return err
	} else {
		cmd := exec.Command(name, "-D", p.dataDir, "--source-server", connstr)
		log.Debugf("execing cmd: %s", cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("error: %v, output: %s", err, string(out))
		}
	}
	return nil
}
