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
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/sorintlab/stolon/internal/common"

	"os"

	"github.com/lib/pq"
)

const (
	// TODO(sgotti) for now we assume wal size is the default 16MiB size
	WalSegSize = (16 * 1024 * 1024) // 16MiB
)

var (
	ValidReplSlotName = regexp.MustCompile("^[a-z0-9_]+$")
)

func dbExec(ctx context.Context, db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
	return db.ExecContext(ctx, query, args...)
}

func query(ctx context.Context, db *sql.DB, query string, args ...interface{}) (*sql.Rows, error) {
	return db.QueryContext(ctx, query, args...)
}

func ping(ctx context.Context, connParams ConnParams) error {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = dbExec(ctx, db, "select 1")
	if err != nil {
		return err
	}
	return nil
}

func setPassword(ctx context.Context, connParams ConnParams, username, password string) error {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("set local log_statement = %s", pq.QuoteLiteral("none"))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}

	query = fmt.Sprintf("alter role %s with encrypted password %s", pq.QuoteIdentifier(username), pq.QuoteLiteral(password))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func createRole(ctx context.Context, connParams ConnParams, roles []string, username, password string) error {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("set local log_statement = %s", pq.QuoteLiteral("none"))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}

	query = fmt.Sprintf("create role %s with login replication encrypted password %s", pq.QuoteIdentifier(username), pq.QuoteLiteral(password))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func createPasswordlessRole(ctx context.Context, connParams ConnParams, roles []string, username string) error {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = dbExec(ctx, db, fmt.Sprintf(`create role "%s" with login replication;`, username))
	return err
}

func alterRole(ctx context.Context, connParams ConnParams, roles []string, username, password string) error {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("set local log_statement = %s", pq.QuoteLiteral("none"))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}

	query = fmt.Sprintf("alter role %s with login replication encrypted password %s", pq.QuoteIdentifier(username), pq.QuoteLiteral(password))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func alterPasswordlessRole(ctx context.Context, connParams ConnParams, roles []string, username string) error {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = dbExec(ctx, db, fmt.Sprintf(`alter role "%s" with login replication;`, username))
	return err
}

// getReplicatinSlots return existing replication slots. On PostgreSQL > 10 we
// skip temporary slots.
func getReplicationSlots(ctx context.Context, connParams ConnParams, maj int) ([]string, error) {
	var q string
	if maj < 10 {
		q = "select slot_name from pg_replication_slots"
	} else {
		q = "select slot_name from pg_replication_slots where temporary is false"
	}

	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	replSlots := []string{}

	rows, err := query(ctx, db, q)
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

func createReplicationSlot(ctx context.Context, connParams ConnParams, name string) error {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = dbExec(ctx, db, fmt.Sprintf("select pg_create_physical_replication_slot('%s')", name))
	return err
}

func dropReplicationSlot(ctx context.Context, connParams ConnParams, name string) error {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = dbExec(ctx, db, fmt.Sprintf("select pg_drop_replication_slot('%s')", name))
	return err
}

func getSyncStandbys(ctx context.Context, connParams ConnParams) ([]string, error) {
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := query(ctx, db, "select application_name, sync_state from pg_stat_replication")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	syncStandbys := []string{}
	for rows.Next() {
		var applicationName, syncState string
		if err := rows.Scan(&applicationName, &syncState); err != nil {
			return nil, err
		}

		if syncState == "sync" {
			syncStandbys = append(syncStandbys, applicationName)
		}
	}

	return syncStandbys, nil
}

func PGLsnToInt(lsn string) (uint64, error) {
	parts := strings.Split(lsn, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("bad pg_lsn: %s", lsn)
	}
	a, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil {
		return 0, err
	}
	b, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		return 0, err
	}
	v := uint64(a)<<32 | b
	return v, nil
}

func GetSystemData(ctx context.Context, replConnParams ConnParams) (*SystemData, error) {
	// Add "replication=1" connection option
	replConnParams["replication"] = "1"
	db, err := sql.Open("postgres", replConnParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := query(ctx, db, "IDENTIFY_SYSTEM")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var sd SystemData
		var xLogPosLsn string
		var unused *string
		if err = rows.Scan(&sd.SystemID, &sd.TimelineID, &xLogPosLsn, &unused); err != nil {
			return nil, err
		}
		sd.XLogPos, err = PGLsnToInt(xLogPosLsn)
		if err != nil {
			return nil, err
		}
		return &sd, nil
	}
	return nil, fmt.Errorf("query returned 0 rows")
}

func parseTimelinesHistory(contents string) ([]*TimelineHistory, error) {
	tlsh := []*TimelineHistory{}
	regex, err := regexp.Compile(`(\S+)\s+(\S+)\s+(.*)$`)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(contents))
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		m := regex.FindStringSubmatch(scanner.Text())
		if len(m) == 4 {
			var tlh TimelineHistory
			if tlh.TimelineID, err = strconv.ParseUint(m[1], 10, 64); err != nil {
				return nil, fmt.Errorf("cannot parse timelineID in timeline history line %q: %v", scanner.Text(), err)
			}
			if tlh.SwitchPoint, err = PGLsnToInt(m[2]); err != nil {
				return nil, fmt.Errorf("cannot parse start lsn in timeline history line %q: %v", scanner.Text(), err)
			}
			tlh.Reason = m[3]
			tlsh = append(tlsh, &tlh)
		}
	}
	return tlsh, err
}

func getTimelinesHistory(ctx context.Context, timeline uint64, replConnParams ConnParams) ([]*TimelineHistory, error) {
	// Add "replication=1" connection option
	replConnParams["replication"] = "1"
	db, err := sql.Open("postgres", replConnParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := query(ctx, db, fmt.Sprintf("TIMELINE_HISTORY %d", timeline))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var timelineFile string
		var contents string
		if err := rows.Scan(&timelineFile, &contents); err != nil {
			return nil, err
		}
		tlsh, err := parseTimelinesHistory(contents)
		if err != nil {
			return nil, err
		}
		return tlsh, nil
	}
	return nil, fmt.Errorf("query returned 0 rows")
}

func IsValidReplSlotName(name string) bool {
	return ValidReplSlotName.MatchString(name)
}

func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func expand(s, dataDir string) string {
	buf := make([]byte, 0, 2*len(s))
	// %d %% are all ASCII, so bytes are fine for this operation.
	i := 0
	for j := 0; j < len(s); j++ {
		if s[j] == '%' && j+1 < len(s) {
			switch s[j+1] {
			case 'd':
				buf = append(buf, s[i:j]...)
				buf = append(buf, []byte(dataDir)...)
				j += 1
				i = j + 1
			case '%':
				j += 1
				buf = append(buf, s[i:j]...)
				i = j + 1
			default:
			}
		}
	}
	return string(buf) + s[i:]
}

func getConfigFilePGParameters(ctx context.Context, connParams ConnParams) (common.Parameters, error) {
	var pgParameters = common.Parameters{}
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// We prefer pg_file_settings since pg_settings returns archive_command = '(disabled)' when archive_mode is off so we'll lose its value
	// Check if pg_file_settings exists (pg >= 9.5)
	rows, err := query(ctx, db, "select 1 from information_schema.tables where table_schema = 'pg_catalog' and table_name = 'pg_file_settings'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := 0
	for rows.Next() {
		c++
	}
	use_pg_file_settings := false
	if c > 0 {
		use_pg_file_settings = true
	}

	if use_pg_file_settings {
		// NOTE If some pg_parameters that cannot be changed without a restart
		// are removed from the postgresql.conf file the view will contain some
		// rows with null name and setting and the error field set to the cause.
		// So we have to filter out these or the Scan will fail.
		rows, err = query(ctx, db, "select name, setting from pg_file_settings where name IS NOT NULL and setting IS NOT NULL")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var name, setting string
			if err = rows.Scan(&name, &setting); err != nil {
				return nil, err
			}
			pgParameters[name] = setting
		}
		return pgParameters, nil
	}

	// Fallback to pg_settings
	rows, err = query(ctx, db, "select name, setting, source from pg_settings")
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

func isRestartRequiredUsingPendingRestart(ctx context.Context, connParams ConnParams) (bool, error) {
	isRestartRequired := false
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return isRestartRequired, err
	}
	defer db.Close()

	rows, err := query(ctx, db, "select count(*) > 0 from pg_settings where pending_restart;")
	if err != nil {
		return isRestartRequired, err
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&isRestartRequired); err != nil {
			return isRestartRequired, err
		}
	}

	return isRestartRequired, nil
}

func isRestartRequiredUsingPgSettingsContext(ctx context.Context, connParams ConnParams, changedParams []string) (bool, error) {
	isRestartRequired := false
	db, err := sql.Open("postgres", connParams.ConnString())
	if err != nil {
		return isRestartRequired, err
	}
	defer db.Close()

	stmt, err := db.Prepare("select count(*) > 0 from pg_settings where context = 'postmaster' and name = ANY($1)")

	if err != nil {
		return false, err
	}

	rows, err := stmt.Query(pq.Array(changedParams))
	if err != nil {
		return isRestartRequired, err
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&isRestartRequired); err != nil {
			return isRestartRequired, err
		}
	}

	return isRestartRequired, nil
}

func ParseBinaryVersion(v string) (int, int, error) {
	// extract version (removing beta*, rc* etc...)
	regex, err := regexp.Compile(`.* \(PostgreSQL\) ([0-9\.]+).*`)
	if err != nil {
		return 0, 0, err
	}
	m := regex.FindStringSubmatch(v)
	if len(m) != 2 {
		return 0, 0, fmt.Errorf("failed to parse postgres binary version: %q", v)
	}
	return ParseVersion(m[1])
}

func ParseVersion(v string) (int, int, error) {
	parts := strings.Split(v, ".")
	if len(parts) < 1 {
		return 0, 0, fmt.Errorf("bad version: %q", v)
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse major %q: %v", parts[0], err)
	}
	min := 0
	if len(parts) > 1 {
		min, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse minor %q: %v", parts[1], err)
		}
	}

	return maj, min, nil
}

func IsWalFileName(name string) bool {
	walChars := "0123456789ABCDEF"
	if len(name) != 24 {
		return false
	}
	for _, c := range name {
		ok := false
		for _, v := range walChars {
			if c == v {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func XlogPosToWalFileNameNoTimeline(XLogPos uint64) string {
	id := uint32(XLogPos >> 32)
	offset := uint32(XLogPos)
	// TODO(sgotti) for now we assume wal size is the default 16M size
	seg := offset / WalSegSize
	return fmt.Sprintf("%08X%08X", id, seg)
}

func WalFileNameNoTimeLine(name string) (string, error) {
	if !IsWalFileName(name) {
		return "", fmt.Errorf("bad wal file name")
	}
	return name[8:24], nil
}
