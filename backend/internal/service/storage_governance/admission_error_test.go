package storagegov

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestAdmissionMissingSchemaUnavailableAndWorkerAreDifferent(t *testing.T) {
	db, control, _ := ownedControl(t, false)
	control.opts.Queue = nil
	require.Equal(t, "STORAGE_CONTROL_WORKER_UNAVAILABLE", dto.CodeOf(control.CheckReady(context.Background())))
	require.NoError(t, db.Migrator().DropTable(&m.ControlRevision{}))
	require.Equal(t, "STORAGE_CONTROL_SCHEMA_NOT_READY", dto.CodeOf(control.CheckReady(context.Background())))
	pool, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	require.Equal(t, "STORAGE_GOVERNANCE_UNAVAILABLE", dto.CodeOf(control.CheckReady(context.Background())))
}

func TestAdmissionUnderlyingErrorClassificationAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{&database.StorageGovernanceSchemaError{Reason: "table_missing"}, "STORAGE_CONTROL_SCHEMA_NOT_READY"},
		{&database.StorageGovernanceSchemaError{Unavailable: true, Reason: "table_inspection_failed"}, "STORAGE_GOVERNANCE_UNAVAILABLE"},
		{&pgconn.PgError{Code: "42P01", Message: "private-sql"}, "STORAGE_CONTROL_SCHEMA_NOT_READY"},
		{&pgconn.PgError{Code: "42703", Message: "private-sql"}, "STORAGE_CONTROL_SCHEMA_NOT_READY"},
		{&pgconn.PgError{Code: "08006", Message: "private-sql"}, "STORAGE_GOVERNANCE_UNAVAILABLE"},
		{errors.New("password=private-sql"), "STORAGE_GOVERNANCE_UNAVAILABLE"},
	} {
		err := admissionUnavailable(tc.err)
		require.Equal(t, tc.code, dto.CodeOf(err))
		require.NotContains(t, err.Error(), "private-sql")
	}
}
