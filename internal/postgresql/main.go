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

// Package postgresql has all stolon specific postgresql code
package postgresql

import (
	"bufio"
	"context"

	// TODO: replace with jackc
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/Masterminds/semver/v3"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/logging"
	"github.com/rs/zerolog"

	// TODO: This can probably go
	"github.com/lib/pq"
)

const (
	postgresConf           = "postgresql.conf"
	postgresRecoveryConf   = "recovery.conf"
	postgresStandbySignal  = "standby.signal"
	postgresRecoverySignal = "recovery.signal"
	postgresRecoveryDone   = "recovery.done"
	postgresAutoConf       = "postgresql.auto.conf"
	tmpPostgresConf        = "stolon-temp-postgresql.conf"

	startTimeout                  = 60 * time.Second
	exitStatusNotRunning          = 3
	exitStatusInaccessibleDatadir = 4

	argDatadir = "-D"
	argDbName  = "-d"
	logExec    = "execing cmd"
)

var (
	// ErrInaccessibleDatadir is raised when pg_ctl returns an unknown state
	ErrInaccessibleDatadir = errors.New("unknown postgres state")

	utlCtx    context.Context
	utlLogger *zerolog.Logger
)

func init() {
	utlCtx, utlLogger = logging.GetLogComponent(context.Background(), logging.PgUtilsComponent)
}

// TODO: implement all other options as well

// ConnParamKey is an enum for PostgreSQL connection parameters
type ConnParamKey string

const (
	// ConnParamKeyHost defines the key for the hostname
	ConnParamKeyHost ConnParamKey = "host"
	// ConnParamKeyPort defines the key for the port
	ConnParamKeyPort ConnParamKey = "port"
	// ConnParamKeyUser defines the key for the username
	ConnParamKeyUser ConnParamKey = "user"
	// ConnParamKeyPassword defines the key for the password
	ConnParamKeyPassword ConnParamKey = "password"
	// ConnParamKeyDbName defines the key for the database name
	ConnParamKeyDbName ConnParamKey = "dbname"
	// ConnParamKeyAppName defines the key for the application name
	ConnParamKeyAppName ConnParamKey = "application_name"
	// ConnParamKeySSLMode defines the key for the ssl mode
	ConnParamKeySSLMode ConnParamKey = "sslmode"
)

// scanner implements a tokenizer for libpq-style option strings.
type scanner struct {
	s []rune
	i int
}

// newScanner returns a new scanner initialized with the option string s.
func newScanner(s string) *scanner {
	return &scanner{[]rune(s), 0}
}

// Next returns the next rune.
// It returns 0, false if the end of the text has been reached.
func (s *scanner) Next() (rune, bool) {
	if s.i >= len(s.s) {
		return 0, false
	}
	r := s.s[s.i]
	s.i++
	return r, true
}

// SkipSpaces returns the next non-whitespace rune.
// It returns 0, false if the end of the text has been reached.
func (s *scanner) SkipSpaces() (rune, bool) {
	r, ok := s.Next()
	for unicode.IsSpace(r) && ok {
		r, ok = s.Next()
	}
	return r, ok
}

// ParseConnString parses the options from name and adds them to the values.
//
// The parsing code is based on conninfo_parse from libpq's fe-connect.c
func ParseConnString(name string) (ConnParams, error) {
	p := ConnParams{}
	s := newScanner(name)

	for {
		var (
			keyRunes, valRunes []rune
			r                  rune
			ok                 bool
		)

		if r, ok = s.SkipSpaces(); !ok {
			break
		}

		// Scan the key
		for !unicode.IsSpace(r) && r != '=' {
			keyRunes = append(keyRunes, r)
			if r, ok = s.Next(); !ok {
				break
			}
		}

		// Skip any whitespace if we're not at the = yet
		if r != '=' {
			r, ok = s.SkipSpaces()
		}

		// The current character should be =
		if r != '=' || !ok {
			return nil, fmt.Errorf(`missing "=" after %q in connection info string"`, string(keyRunes))
		}

		// Skip any whitespace after the =
		if r, ok = s.SkipSpaces(); !ok {
			// If we reach the end here, the last value is just an empty string as per libpq.
			p.Set(ConnParamKey(ConnParamKey(keyRunes)), "")
			break
		}

		if r != '\'' {
			for !unicode.IsSpace(r) {
				if r == '\\' {
					if r, ok = s.Next(); !ok {
						return nil, errors.New(`missing character after backslash`)
					}
				}
				valRunes = append(valRunes, r)

				if r, ok = s.Next(); !ok {
					break
				}
			}
		} else {
		quote:
			for {
				if r, ok = s.Next(); !ok {
					return nil, errors.New(`unterminated quoted string literal in connection string`)
				}
				switch r {
				case '\'':
					break quote
				case '\\':
					r, _ = s.Next()
					fallthrough
				default:
					valRunes = append(valRunes, r)
				}
			}
		}

		p.Set(ConnParamKey(keyRunes), string(valRunes))
	}

	return p, nil
}

// URLToConnParams creates the connParams from the url.
func URLToConnParams(urlStr string) (ConnParams, error) {
	p := ConnParams{}
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("invalid connection protocol: %s", u.Scheme)
	}

	if u.User != nil {
		v := u.User.Username()
		p.Set("user", v)
		v, _ = u.User.Password()
		p.Set("password", v)
	}

	if host := u.Hostname(); host != "" {
		p.Set("host", host)
	}
	if port := u.Port(); port != "" {
		p.Set("port", port)
	}

	if u.Path != "" {
		p.Set("dbname", u.Path[1:])
	}

	q := u.Query()
	for k := range q {
		p.Set(ConnParamKey(k), q.Get(k))
	}

	return p, nil
}

var (
	// V95 represents PostgreSQL 9.5
	V95 = semver.MustParse("9.5")
	// V96 represents PostgreSQL 9.6
	V96 = semver.MustParse("9.6")
	// V10 represents PostgreSQL 10
	V10 = semver.MustParse("10")
	// V12 represents PostgreSQL 12
	V12 = semver.MustParse("12")
	// V13 represents PostgreSQL 13
	V13 = semver.MustParse("13")
	// V18 represents PostgreSQL 18
	V18 = semver.MustParse("18")
)

func parseBinaryVersion(v string) (*semver.Version, error) {
	// extract version (removing beta*, rc* etc...)

	regex := regexp.MustCompile(`.* \(PostgreSQL\) ([0-9\.]+).*`)
	m := regex.FindStringSubmatch(v)
	if len(m) != 2 {
		return nil, fmt.Errorf("failed to parse postgres binary version: %q", v)
	}
	return semver.NewVersion(m[1])
}

func parseVersion(v string) (*semver.Version, error) {
	return semver.NewVersion(v)
}

func pgDataVersion(dataDir string) (*semver.Version, error) {
	fh, err := os.Open(filepath.Join(dataDir, "PG_VERSION"))
	if err != nil {
		return nil, fmt.Errorf("failed to read PG_VERSION: %v", err)
	}
	defer handledFileClose(fh)

	scanner := bufio.NewScanner(fh)
	scanner.Split(bufio.ScanLines)

	scanner.Scan()

	version := scanner.Text()
	return parseVersion(version)
}

func binaryVersion(binPath string) (*semver.Version, error) {
	name := filepath.Join(binPath, "postgres")
	cmd := exec.Command(name, "-V")
	utlLogger.Debug().Str(logCmd, cmd.String()).Msg("execing cmd")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error: %v, output: %s", err, string(out))
	}
	return parseBinaryVersion(string(out))
}

const (
	// TODO: can we autodetect if this is non-default?
	walSegSize = (16 * 1024 * 1024) // 16MiB
	globalDB   = "postgres"

	urwx = 0o700
	urw  = 0o600

	logCmd = "cmd"

	base16    = 16
	base10    = 10
	bitSize32 = 32
	bitSize64 = 64

	historyTimelineStringLength  = 4
	fileNameTimelineStringLength = 8
	walNameLength                = 24
)

var (
	validReplSlotName = regexp.MustCompile("^[a-z0-9_]+$")
)

func handledDbClose(db *sql.DB) {
	if err := db.Close(); err != nil {
		utlLogger.Fatal().AnErr("err", err).Msg("Failed to close db connection")
	}
}

func handledRowsClose(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		utlLogger.Fatal().AnErr("err", err).Msg("Failed to close cursor")
	}
}

func handledFileClose(fh *os.File) {
	if err := fh.Close(); err != nil {
		utlLogger.Fatal().Str("file", fh.Name()).AnErr("err", err).Msg("Failed to close")
	}
}

func handledDBClose(fh *sql.DB) {
	if err := fh.Close(); err != nil {
		utlLogger.Fatal().AnErr("err", err).Msg("Failed to close database")
	}
}

func handledFileRemove(fh *os.File) {
	if err := os.Remove(fh.Name()); err != nil {
		utlLogger.Fatal().Str("path", fh.Name()).AnErr("err", err).Msg("Failed to remove")
	}
	handledFileClose(fh)
}

func dbExec(ctx context.Context, db *sql.DB, query string, args ...any) (sql.Result, error) {
	return db.ExecContext(ctx, query, args...)
}

func query(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(ctx, query, args...)
}

func ping(ctx context.Context, connParams ConnParams) error {
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return err
	}
	defer handledDBClose(db)

	_, err = dbExec(ctx, db, "select 1")
	if err != nil {
		return err
	}
	return nil
}

func setPassword(ctx context.Context, connParams ConnParams, username, password string) error {
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return err
	}
	defer handledDbClose(db)

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("set local log_statement = %s", pq.QuoteLiteral("none"))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}

	query = fmt.Sprintf(
		"alter role %s with encrypted password %s",
		pq.QuoteIdentifier(username),
		pq.QuoteLiteral(password),
	)
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// TODO: remove _ parameters

func createRole(ctx context.Context, connParams ConnParams, _ []string, username, password string) error {
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return err
	}
	defer handledDbClose(db)

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("set local log_statement = %s", pq.QuoteLiteral("none"))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}

	query = fmt.Sprintf(
		"create role %s with login replication encrypted password %s",
		pq.QuoteIdentifier(username),
		pq.QuoteLiteral(password),
	)
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func createPasswordlessRole(ctx context.Context, connParams ConnParams, _ []string, username string) error {
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return err
	}
	defer handledDbClose(db)

	_, err = dbExec(ctx, db, fmt.Sprintf(`create role "%s" with login replication;`, username))
	return err
}

func alterRole(ctx context.Context, connParams ConnParams, _ []string, username, password string) error {
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return err
	}
	defer handledDbClose(db)

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("set local log_statement = %s", pq.QuoteLiteral("none"))
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}

	query = fmt.Sprintf(
		"alter role %s with login replication encrypted password %s",
		pq.QuoteIdentifier(username),
		pq.QuoteLiteral(password),
	)
	if _, err = tx.ExecContext(ctx, query); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func alterPasswordlessRole(ctx context.Context, connParams ConnParams, _ []string, username string) error {
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return err
	}
	defer handledDbClose(db)

	_, err = dbExec(ctx, db, fmt.Sprintf(`alter role "%s" with login replication;`, username))
	return err
}

// getReplicatinSlots return existing replication slots. On PostgreSQL > 10 we
// skip temporary slots.
func getReplicationSlots(ctx context.Context, connParams ConnParams, version *semver.Version) ([]string, error) {
	var q string
	if version.LessThan(V10) {
		q = "select slot_name from pg_replication_slots"
	} else {
		q = "select slot_name from pg_replication_slots where temporary is false"
	}

	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer handledDbClose(db)

	replSlots := []string{}

	rows, err := query(ctx, db, q)
	if err != nil {
		return nil, err
	}
	defer handledRowsClose(rows)
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
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return err
	}
	defer handledDbClose(db)

	_, err = dbExec(ctx, db, fmt.Sprintf("select pg_create_physical_replication_slot('%s')", name))
	return err
}

func dropReplicationSlot(ctx context.Context, connParams ConnParams, name string) error {
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return err
	}
	defer handledDbClose(db)

	_, err = dbExec(ctx, db, fmt.Sprintf("select pg_drop_replication_slot('%s')", name))
	return err
}

func getSyncStandbys(ctx context.Context, connParams ConnParams) ([]string, error) {
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer handledDbClose(db)

	rows, err := query(ctx, db, "select application_name, sync_state from pg_stat_replication")
	if err != nil {
		return nil, err
	}
	defer handledRowsClose(rows)

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

// PGLsnToInt will return an uint64 representing an absolute byte in the WAL stream
func PGLsnToInt(lsn string) (uint64, error) {
	parts := strings.Split(lsn, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("bad pg_lsn: %s", lsn)
	}
	a, err := strconv.ParseUint(parts[0], base16, bitSize32)
	if err != nil {
		return 0, err
	}
	b, err := strconv.ParseUint(parts[1], base16, bitSize32)
	if err != nil {
		return 0, err
	}
	v := uint64(a)<<bitSize32 | b
	return v, nil
}

// GetSystemData returns the postgreSQL system data (IDENTIFY_SYSTEM)
func GetSystemData(ctx context.Context, replConnParams ConnParams) (*SystemData, error) {
	// Add "replication=1" connection option
	replConnParams["replication"] = "1"
	db, err := sql.Open(globalDB, replConnParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer handledDbClose(db)

	rows, err := query(ctx, db, "IDENTIFY_SYSTEM")
	if err != nil {
		return nil, err
	}
	defer handledRowsClose(rows)
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
	return nil, errors.New("query returned 0 rows")
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
		if len(m) == historyTimelineStringLength {
			var tlh TimelineHistory
			if tlh.TimelineID, err = strconv.ParseUint(m[1], base10, bitSize64); err != nil {
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
	db, err := sql.Open(globalDB, replConnParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer handledDbClose(db)

	rows, err := query(ctx, db, fmt.Sprintf("TIMELINE_HISTORY %d", timeline))
	if err != nil {
		return nil, err
	}
	defer handledRowsClose(rows)
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
	return nil, errors.New("query returned 0 rows")
}

// IsValidReplSlotName validates if a string can be used as a name for a replication slot
func IsValidReplSlotName(name string) bool {
	return validReplSlotName.MatchString(name)
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

// expandRecoveryCommand substitues the data and wal directories into a point-in-time
// recovery command string. Any %d become the data directory, any %w become the wal
// directory and any literal % characters are escaped by themselves (%% -> %).
func expandRecoveryCommand(cmd, dataDir, walDir string) string {
	return regexp.MustCompile(`%[dw%]`).ReplaceAllStringFunc(cmd, func(match string) string {
		switch match[1] {
		case 'd':
			return dataDir
		case 'w':
			return walDir
		}

		return "%"
	})
}

func getConfigFilePGParameters(ctx context.Context, connParams ConnParams) (common.Parameters, error) {
	var pgParameters = common.Parameters{}
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return nil, err
	}
	defer handledDbClose(db)

	// We prefer pg_file_settings since pg_settings returns archive_command = '(disabled)'
	// when archive_mode is off so we'll lose its value
	// Check if pg_file_settings exists (pg >= 9.5)
	rows, err := query(
		ctx,
		db,
		strings.Join([]string{
			"select 1 from information_schema.tables",
			"where table_schema = 'pg_catalog' ",
			"and table_name = 'pg_file_settings'",
		}, "\n"))
	if err != nil {
		return nil, err
	}
	defer handledRowsClose(rows)
	c := 0
	for rows.Next() {
		c++
	}
	usePGFileSettings := false
	if c > 0 {
		usePGFileSettings = true
	}

	if usePGFileSettings {
		// NOTE If some pg_parameters that cannot be changed without a restart
		// are removed from the postgresql.conf file the view will contain some
		// rows with null name and setting and the error field set to the cause.
		// So we have to filter out these or the Scan will fail.
		rows, err = query(
			ctx,
			db,
			strings.Join([]string{
				"select name, setting",
				"from pg_file_settings",
				"where name IS NOT NULL",
				"and setting IS NOT NULL",
			}, "\n"))
		if err != nil {
			return nil, err
		}
		defer handledRowsClose(rows)
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
	defer handledRowsClose(rows)
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
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return isRestartRequired, err
	}
	defer handledDbClose(db)

	rows, err := query(ctx, db, "select count(*) > 0 from pg_settings where pending_restart;")
	if err != nil {
		return isRestartRequired, err
	}
	defer handledRowsClose(rows)
	if rows.Next() {
		if err := rows.Scan(&isRestartRequired); err != nil {
			return isRestartRequired, err
		}
	}

	return isRestartRequired, nil
}

func isRestartRequiredUsingPgSettingsContext(
	_ context.Context,
	connParams ConnParams,
	changedParams []string,
) (bool, error) {
	isRestartRequired := false
	db, err := sql.Open(globalDB, connParams.ConnString())
	if err != nil {
		return isRestartRequired, err
	}
	defer handledDbClose(db)

	stmt, err := db.Prepare("select count(*) > 0 from pg_settings where context = 'postmaster' and name = ANY($1)")

	if err != nil {
		return false, err
	}

	rows, err := stmt.Query(pq.Array(changedParams))
	if err != nil {
		return isRestartRequired, err
	}
	defer handledRowsClose(rows)
	if rows.Next() {
		if err := rows.Scan(&isRestartRequired); err != nil {
			return isRestartRequired, err
		}
	}

	return isRestartRequired, nil
}

// IsWalFileName checks if a file name is the name of a WAL file
func IsWalFileName(name string) bool {
	walChars := "0123456789ABCDEF"
	if len(name) != walNameLength {
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

// XlogPosToWalFileNameNoTimeline can be used to convert a WAL location to a WAL file name
func XlogPosToWalFileNameNoTimeline(xLogPos uint64) string {
	id := uint32(xLogPos >> bitSize32)
	offset := uint32(xLogPos)
	// TODO(sgotti) for now we assume wal size is the default 16M size
	seg := offset / walSegSize
	return fmt.Sprintf("%08X%08X", id, seg)
}

// WalFileNameNoTimeLine returns the absolute byte from a WAL stream that a WAL file belongs to
func WalFileNameNoTimeLine(name string) (string, error) {
	if !IsWalFileName(name) {
		return "", errors.New("bad wal file name")
	}
	return name[fileNameTimelineStringLength:walNameLength], nil
}

func moveFile(sourcePath, destPath string) error {
	// using os.Rename is faster when on same filesystem
	if err := os.Rename(sourcePath, destPath); err == nil {
		return nil
	}
	// Error. Let's try to write
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("Couldn't open source file: %s", err)
	}
	inFileStat, err := inputFile.Stat()
	if err != nil {
		return err
	}
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	perm := inFileStat.Mode() & os.ModePerm
	outputFile, err := os.OpenFile(destPath, flag, perm)
	if err != nil {
		return err
	}
	defer handledFileClose(outputFile)
	_, err = io.Copy(outputFile, inputFile)
	handledFileClose(inputFile)
	if err != nil {
		return fmt.Errorf("Writing to output file failed: %s", err)
	}
	// The copy was successful, so now delete the original file
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("Failed removing original file: %s", err)
	}
	return nil
}

func moveDirRecursive(ctx context.Context, src string, dest string) (err error) {
	_, logger := logging.GetLogComponent(ctx, logging.PgComponent)
	var stat fs.FileInfo
	logger.Info().Str("src", src).Str("dest", dest).Msg("Moving")
	if stat, err = os.Stat(src); err != nil {
		logger.Error().Str("path", src).AnErr("err", err).Msg("could not get stat of file")
		return err
	} else if !stat.IsDir() {
		return moveFile(src, dest)
	}
	// Make the dir if it doesn't exist
	if _, err = os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(dest, stat.Mode()&os.ModePerm); err != nil {
			return err
		}
	} else if err != nil {
		logger.Error().Str("path", dest).AnErr("err", err).Msg("could not get stat of file")
		return err
	}
	// Copy all files and folders in this folder
	var entries []fs.DirEntry
	if entries, err = os.ReadDir(src); err != nil {
		logger.Error().Str("path", src).AnErr("err", err).Msg("could not read contents of folder")
		return err
	}
	for _, entry := range entries {
		srcEntry := filepath.Join(src, entry.Name())
		dstEntry := filepath.Join(dest, entry.Name())
		if err := moveDirRecursive(ctx, srcEntry, dstEntry); err != nil {
			return err
		}
	}

	// Remove this folder, which is now supposedly empty
	if err := syscall.Rmdir(src); err != nil {
		logger.Error().Str("path", src).AnErr("err", err).Msg("could not remove folder")
		// If this is a mountpoint or you don't have enough permissions, you might nog be able to. But that is fine.
		// return err
	}
	return nil
}
