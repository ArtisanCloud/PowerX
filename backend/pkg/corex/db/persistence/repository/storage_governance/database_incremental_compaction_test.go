package storagegovrepo

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"testing/synctest"
	"time"

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

func TestIncrementalWindowFollowsBudgetAndMeasuredReclaim(t *testing.T) {
	require.EqualValues(t, 1024, incrementalPageLimit(32<<20, 1<<30))
	require.EqualValues(t, 8192, incrementalPageLimit(20<<30, 256<<20))
	require.EqualValues(t, 16384, incrementalPageLimit(20<<30, 384<<20))
	net := int64(-64 << 20)
	b := MaintenanceIncrementalBatch{VacuumAttempts: 1, HeapReclaimedBytes: 64 << 20, TableChangeBytes: &net}
	require.EqualValues(t, 16384, growIncrementalWindow(8192, 16384, b))
	require.EqualValues(t, 8192, growIncrementalWindow(8192, 8192, b))
	b.VacuumAttempts = 2
	require.EqualValues(t, 4096, growIncrementalWindow(8192, 16384, b))
}

func TestIncrementalTailExcludesProtectedPage(t *testing.T) {
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
	_, err = conn.ExecContext(ctx, `CREATE TEMP TABLE suf_probe (record_class varchar, completed_at timestamptz, deleted_at timestamptz, status varchar, request_payload text, response_payload text)`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO suf_probe VALUES ('legacy_unclassified',now(),NULL,'success',NULL,NULL)`)
	require.NoError(t, err)
	table := MaintenanceTable{ID: incrementalAuditTable}
	// A protected row on the final page still prevents any move. With an
	// empty suffix no earlier FSM capacity is required, even if the protected page is full.
	_, err = incrementalTailStart(ctx, conn, "pg_temp.suf_probe", "unused", table, 0, 1)
	require.ErrorIs(t, err, ErrIncrementalProtected)
	_, err = conn.ExecContext(ctx, `ALTER TABLE suf_probe ALTER COLUMN request_payload SET STORAGE PLAIN`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO suf_probe SELECT 'capability_invocation_audit',now(),NULL,'success',repeat('x',6000),NULL FROM generate_series(1,2000)`)
	require.NoError(t, err)
	var end int64
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT pg_relation_size('pg_temp.suf_probe')/8192`).Scan(&end))
	_, err = conn.ExecContext(ctx, `DELETE FROM suf_probe WHERE record_class='capability_invocation_audit'`)
	require.NoError(t, err)
	start, err := incrementalTailStart(ctx, conn, "pg_temp.suf_probe", "unused", table, 0, end)
	require.NoError(t, err)
	require.EqualValues(t, 1, start)
	_, err = conn.ExecContext(ctx, `DELETE FROM suf_probe`)
	require.NoError(t, err)
	start, err = incrementalTailStart(ctx, conn, "pg_temp.suf_probe", "unused", table, 0, end)
	require.NoError(t, err)
	require.Zero(t, start)
}

func TestIncrementalContinuesAfterRoundTimeBudget(t *testing.T) {
	for _, tt := range []struct {
		name       string
		continuous bool
		targetAt   int
		wantRounds []int
		wantStop   string
	}{
		{"legacy request", false, 0, []int{0, 0}, "time_budget"},
		{"continue to target", true, 3, []int{1, 1, 2}, "target_reached"},
		{"continue until approved window", true, 0, []int{1, 1, 2}, "time_budget"},
		{"target already reached", true, -1, nil, "target_reached"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 19*time.Minute)
				defer cancel()
				var rounds []int
				var reason string
				var deadlines []time.Time
				targetReached := func(context.Context) (bool, error) {
					return tt.targetAt == -1 || tt.targetAt > 0 && tt.targetAt == len(rounds), nil
				}
				runSlice := func(ctx context.Context, _ *MaintenanceIncrementalBatch) error {
					deadline, ok := ctx.Deadline()
					require.True(t, ok)
					deadlines = append(deadlines, deadline)
					time.Sleep(5 * time.Minute)
					return ctx.Err()
				}
				report := func(report MaintenanceIncrementalReport) error {
					if report.Batch != nil {
						require.EqualValues(t, (5 * time.Minute).Milliseconds(), report.Batch.ElapsedMS)
						rounds = append(rounds, report.Batch.Round)
					} else {
						reason = report.StopReason
					}
					return nil
				}
				require.NoError(t, runIncrementalRounds(ctx, 64, tt.continuous, targetReached, runSlice, report))
				require.Equal(t, tt.wantRounds, rounds)
				require.Equal(t, tt.wantStop, reason)
				if len(deadlines) == 3 {
					windowEnd, _ := ctx.Deadline()
					require.Equal(t, deadlines[0], deadlines[1])
					require.Equal(t, windowEnd, deadlines[2])
				}
			})
		})
	}
}

func TestIncrementalDoesNotContinueAfterBatchProtection(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
	defer cancel()
	attempts := 0
	err := runIncrementalRounds(ctx, 1, true, func(context.Context) (bool, error) { return false, nil },
		func(_ context.Context, batch *MaintenanceIncrementalBatch) error {
			attempts++
			batch.StopPhase = "vacuum"
			return ErrIncrementalNoReclaim
		}, func(MaintenanceIncrementalReport) error {
			t.Fatal("an unsuccessful batch cannot schedule another round")
			return nil
		})
	require.ErrorIs(t, err, ErrIncrementalNoReclaim)
	require.Equal(t, 1, attempts)
}
