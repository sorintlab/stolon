// Copyright 2016 Sorint.lab
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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/satori/go.uuid"
	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"
	"github.com/sorintlab/stolon/pkg/store"
)

func TestPITR(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "stolon")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.RemoveAll(dir)

	baseBackupDir, err := ioutil.TempDir(dir, "basebackup")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	archiveBackupDir, err := ioutil.TempDir(dir, "archivebackup")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tstore := setupStore(t, dir)
	defer tstore.Stop()

	storeEndpoints := fmt.Sprintf("%s:%s", tstore.listenAddress, tstore.port)

	clusterName := uuid.NewV4().String()

	storePath := filepath.Join(common.StorePrefix, clusterName)

	sm := store.NewKVBackedStore(tstore.store, storePath)

	initialClusterSpec := &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModeNew),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		PGParameters: pgParametersWithDefaults(cluster.PGParameters{
			"archive_mode":    "on",
			"archive_command": fmt.Sprintf("cp %%p %s/%%f", archiveBackupDir),
		}),
	}
	initialClusterSpecFile, err := writeClusterSpec(dir, initialClusterSpec)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tk, err := NewTestKeeper(t, dir, clusterName, pgSUUsername, pgSUPassword, pgReplUsername, pgReplPassword, tstore.storeBackend, storeEndpoints)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer tk.Stop()

	ts, err := NewTestSentinel(t, dir, clusterName, tstore.storeBackend, storeEndpoints, fmt.Sprintf("--initial-cluster-spec=%s", initialClusterSpecFile))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := ts.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Wait for clusterView containing a master
	_, err = WaitClusterDataWithMaster(sm, 30*time.Second)
	if err != nil {
		t.Fatal("expected a master in cluster view")
	}
	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitDBRole(common.RoleMaster, nil, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := populate(t, tk); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := write(t, tk, 2, 2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// ioutil.Tempfile already creates files with 0600 permissions
	pgpass, err := ioutil.TempFile("", "pgpass")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	pgpass.WriteString(fmt.Sprintf("%s:%s:*:%s:%s\n", tk.pgListenAddress, tk.pgPort, tk.pgReplUsername, tk.pgReplPassword))
	// Don't save the wal during the basebackup (-x). This to test that archive_command and restore command correctly work.
	cmd := exec.Command("pg_basebackup", "-F", "tar", "-D", baseBackupDir, "-h", tk.pgListenAddress, "-p", tk.pgPort, "-U", tk.pgReplUsername)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PGPASSFILE=%s", pgpass.Name()))
	t.Logf("execing cmd: %v", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("error: %v, output: %s", err, string(out))
	}

	// Switch wal so they will be archived
	if err := tk.SwitchWals(1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	ts.Stop()

	// Delete the current cluster data
	if err := tstore.store.Delete(context.TODO(), filepath.Join(storePath, "clusterdata")); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Delete sentinel leader key to just speedup new election
	if err := tstore.store.Delete(context.TODO(), filepath.Join(storePath, common.SentinelLeaderKey)); err != nil && err != store.ErrKeyNotFound {
		t.Fatalf("unexpected err: %v", err)
	}

	// Now initialize a new cluster with the existing keeper
	initialClusterSpec = &cluster.ClusterSpec{
		InitMode:           cluster.ClusterInitModeP(cluster.ClusterInitModePITR),
		SleepInterval:      &cluster.Duration{Duration: 2 * time.Second},
		FailInterval:       &cluster.Duration{Duration: 5 * time.Second},
		ConvergenceTimeout: &cluster.Duration{Duration: 30 * time.Second},
		PITRConfig: &cluster.PITRConfig{
			DataRestoreCommand: fmt.Sprintf("tar xvf %s/base.tar -C %%d", baseBackupDir),
			ArchiveRecoverySettings: &cluster.ArchiveRecoverySettings{
				RestoreCommand: fmt.Sprintf("cp %s/%%f %%p", archiveBackupDir),
			},
		},
		PGParameters: pgParametersWithDefaults(cluster.PGParameters{
			"max_prepared_transactions": "100",
		}),
	}
	initialClusterSpecFile, err = writeClusterSpec(dir, initialClusterSpec)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	ts, err = NewTestSentinel(t, dir, clusterName, tstore.storeBackend, storeEndpoints, fmt.Sprintf("--initial-cluster-spec=%s", initialClusterSpecFile))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := ts.Start(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer ts.Stop()

	if err := WaitClusterPhase(sm, cluster.ClusterPhaseNormal, 60*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	_, err = WaitClusterDataWithMaster(sm, 30*time.Second)
	if err != nil {
		t.Fatal("expected a master in cluster view")
	}
	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := tk.WaitDBRole(common.RoleMaster, nil, 30*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if err := tk.WaitDBUp(60 * time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	c, err := getLines(t, tk)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c != 1 {
		t.Fatalf("wrong number of lines, want: %d, got: %d", 2, c)
	}

	pgParameters, err := tk.GetPGParameters()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v := pgParameters["max_prepared_transactions"]; v != "100" {
		t.Fatalf("expected max_prepared_transactions == 100 got %q", v)
	}
}
