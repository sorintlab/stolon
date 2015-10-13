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
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/sorintlab/stolon/common"
	"github.com/sorintlab/stolon/pkg/cluster"

	_ "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/lib/pq"
	"github.com/sorintlab/stolon/Godeps/_workspace/src/golang.org/x/net/context"
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

func GetRole(ctx context.Context, connString string) (common.Role, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	rows, err := Query(ctx, db, "SELECT pg_is_in_recovery from pg_is_in_recovery()")
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
		log.Debugf("pgState: %#v", pgState)
		pgState.XLogPos, err = PGLSNToInt(xLogPosLsn)
		if err != nil {
			return nil, err
		}
		return &pgState, nil
	}
	return nil, fmt.Errorf("query returned 0 rows")
}

func GetTimelineHistory(ctx context.Context, timeline uint64, replConnString string) (string, error) {
	// Add "replication=1" connection option
	u, err := url.Parse(replConnString)
	if err != nil {
		return "", err
	}
	v := u.Query()
	v.Add("replication", "1")
	u.RawQuery = v.Encode()
	replConnString = u.String()
	db, err := sql.Open("postgres", replConnString)
	if err != nil {
		return "", err
	}
	defer db.Close()

	log.Debugf("timeline: %d", timeline)
	rows, err := Query(ctx, db, fmt.Sprintf("TIMELINE_HISTORY %d", timeline))
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var timelineFile string
		var contents string
		if err := rows.Scan(&timelineFile, &contents); err != nil {
			return "", err
		}
		return contents, nil
	}
	return "", fmt.Errorf("query returned 0 rows")
}
