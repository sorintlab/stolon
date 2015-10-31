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
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"

	_ "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/lib/pq"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
)

var (
	ValidReplSlotName = regexp.MustCompile("^[a-z0-9_]+$")
)

func Exec(ctx context.Context, db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
	ch := make(chan struct {
		res sql.Result
		err error
	})
	go func() {
		res, err := db.Exec(query, args...)
		ch <- struct {
			res sql.Result
			err error
		}{res, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-ch:
		return out.res, out.err
	}
}

func Query(ctx context.Context, db *sql.DB, query string, args ...interface{}) (*sql.Rows, error) {
	ch := make(chan struct {
		rows *sql.Rows
		err  error
	})
	go func() {
		rows, err := db.Query(query, args...)
		ch <- struct {
			rows *sql.Rows
			err  error
		}{rows, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-ch:
		return out.rows, out.err
	}
}

func CheckDBStatus(ctx context.Context, connString string) error {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = Exec(ctx, db, "select 1")
	if err != nil {
		return err
	}
	return nil
}

func CreateReplRole(ctx context.Context, connString, replUser, replPassword string) error {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = Exec(ctx, db, fmt.Sprintf(`create role "%s" with login replication encrypted password '%s';`, replUser, replPassword))
	return err
}

func GetReplicatinSlots(ctx context.Context, connString string) ([]string, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	replSlots := []string{}

	rows, err := Query(ctx, db, "select slot_name from pg_replication_slots")
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

func CreateReplicationSlot(ctx context.Context, connString string, name string) error {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = Exec(ctx, db, fmt.Sprintf("select pg_create_physical_replication_slot('%s')", name))
	return err
}

func DropReplicationSlot(ctx context.Context, connString string, name string) error {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = Exec(ctx, db, fmt.Sprintf("select pg_drop_replication_slot('%s')", name))
	return err
}

func GetRole(ctx context.Context, connString string) (common.Role, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	rows, err := Query(ctx, db, "select pg_is_in_recovery from pg_is_in_recovery()")
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
			return common.StandbyRole, nil
		}
		return common.MasterRole, nil
	}
	return 0, fmt.Errorf("no rows returned")
}

func GetPGMasterLocation(ctx context.Context, connString string) (uint64, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	rows, err := Query(ctx, db, "select pg_current_xlog_location() - '0/0000000'")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var location uint64
		if err := rows.Scan(&location); err != nil {
			return 0, err
		}
		return location, nil
	}
	return 0, fmt.Errorf("no rows returned")
}

func PGLSNToInt(lsn string) (uint64, error) {
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

func GetPGState(ctx context.Context, replConnString string) (*cluster.PostgresState, error) {
	// Add "replication=1" connection option
	u, err := url.Parse(replConnString)
	if err != nil {
		return nil, err
	}
	v := u.Query()
	v.Add("replication", "1")
	u.RawQuery = v.Encode()
	replConnString = u.String()
	db, err := sql.Open("postgres", replConnString)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := Query(ctx, db, "IDENTIFY_SYSTEM")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pgState cluster.PostgresState
		var xLogPosLsn string
		var unused *string
		if err := rows.Scan(&pgState.SystemID, &pgState.TimelineID, &xLogPosLsn, &unused); err != nil {
			return nil, err
		}
		pgState.XLogPos, err = PGLSNToInt(xLogPosLsn)
		if err != nil {
			return nil, err
		}
		return &pgState, nil
	}
	return nil, fmt.Errorf("query returned 0 rows")
}

func parseTimeLinesHistory(contents string) (cluster.PostgresTimeLinesHistory, error) {
	tlsh := cluster.PostgresTimeLinesHistory{}
	regex, err := regexp.Compile(`(\S+)\s+(\S+)\s+(.*)$`)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(contents))
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		m := regex.FindStringSubmatch(scanner.Text())
		if len(m) == 4 {
			var tlh cluster.PostgresTimeLineHistory
			if tlh.TimelineID, err = strconv.ParseUint(m[1], 10, 64); err != nil {
				return nil, fmt.Errorf("cannot parse timelineID in timeline history line %q: %v", scanner.Text(), err)
			}
			if tlh.SwitchPoint, err = PGLSNToInt(m[2]); err != nil {
				return nil, fmt.Errorf("cannot parse start lsn in timeline history line %q: %v", scanner.Text(), err)
			}
			tlh.Reason = m[3]
			tlsh = append(tlsh, &tlh)
		}
	}
	return tlsh, err
}

func GetTimelinesHistory(ctx context.Context, timeline uint64, replConnString string) (cluster.PostgresTimeLinesHistory, error) {
	// Add "replication=1" connection option
	u, err := url.Parse(replConnString)
	if err != nil {
		return nil, err
	}
	v := u.Query()
	v.Add("replication", "1")
	u.RawQuery = v.Encode()
	replConnString = u.String()
	db, err := sql.Open("postgres", replConnString)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := Query(ctx, db, fmt.Sprintf("TIMELINE_HISTORY %d", timeline))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var timelineFile string
		var contents string
		if err := rows.Scan(&timelineFile, &contents); err != nil {
			return nil, err
		}
		tlsh, err := parseTimeLinesHistory(contents)
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
