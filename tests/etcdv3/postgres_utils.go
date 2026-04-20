package etcdv3_test

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type pgConnParams map[string]string

func (cp pgConnParams) connString() string {
	var connString []string
	for k, v := range cp {
		v = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", `\'`))
		item := fmt.Sprintf("%s=%s", k, v)
		connString = append(connString, item)
	}
	return strings.Join(connString, " ")
}

func (cp pgConnParams) setParam(key, value string) pgConnParams {
	newCp := maps.Clone(cp)
	newCp[key] = value
	return newCp
}

func pgPing(ctx context.Context, connParams pgConnParams) error {
	conn, err := pgx.Connect(ctx, connParams.connString())
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	var name string
	err = conn.QueryRow(ctx, "select datname from pg_database;").Scan(&name)
	if err != nil {
		return err
	}
	return nil
}

func isReady(ctx context.Context, connParams pgConnParams, interval time.Duration) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := pgPing(ctx, connParams); err != nil {
			return nil
		}
		time.Sleep(interval)
	}
}
