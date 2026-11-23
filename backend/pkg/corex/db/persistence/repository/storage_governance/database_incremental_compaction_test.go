package storagegovrepo

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

func TestIncrementalMovePreservesCancellation(t *testing.T) {
	for _, err := range []error{
		context.Canceled,
		context.DeadlineExceeded,
		&pgconn.PgError{Code: "57014", Message: "canceling statement due to user request"},
		&pgconn.PgError{Code: "55P03", Message: "lock not available"},
	} {
		require.ErrorIs(t, incrementalUpdateError(context.Background(), nil, err), err)
	}
}

func TestIncrementalMoveRollsBackBeforeSmallerWindow(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, "CREATE TEMP TABLE move_probe (id int primary key, value int)")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "INSERT INTO move_probe VALUES (1,0),(2,0)")
	require.NoError(t, err)
	tx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "UPDATE move_probe SET value=1 WHERE id=1")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "SET LOCAL statement_timeout='50ms'")
	require.NoError(t, err)
	_, slow := tx.ExecContext(ctx, "UPDATE move_probe SET value=2 FROM pg_sleep(1) WHERE id=2")
	require.Error(t, slow)
	require.ErrorIs(t, incrementalUpdateError(ctx, tx, slow), ErrIncrementalLimit)
	// 已结束的事务无法再确认回滚，不能据此重新执行写入。
	require.ErrorIs(t, incrementalUpdateError(ctx, tx, slow), sql.ErrTxDone)
	var sum int
	err = conn.QueryRowContext(ctx, "SELECT SUM(value) FROM move_probe").Scan(&sum)
	require.NoError(t, err)
	require.Zero(t, sum)
	_, err = conn.ExecContext(ctx, "UPDATE move_probe SET value=2 WHERE id=2")
	require.NoError(t, err)
}
