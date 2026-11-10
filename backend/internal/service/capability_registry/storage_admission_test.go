package capability_registry

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestAdmissionClassificationNeverTreatsDependencyFailureAsCapacity(t *testing.T) {
	for _, code := range []string{"STORAGE_CONTROL_SCHEMA_NOT_READY", "STORAGE_GOVERNANCE_UNAVAILABLE", "STORAGE_CAPACITY_EXHAUSTED", "UNCLASSIFIED_FAILURE"} {
		t.Run(code, func(t *testing.T) {
			const private = "password=private-sentinel SELECT secret FROM internal"
			a := NewAuditService(AuditServiceOptions{})
			a.SetStorageAdmission(func(context.Context) error { return dto.NewErrorWithCode(503, code, private, errors.New(private)) })
			err := a.checkStorageAdmission(context.Background())
			want := code
			if code == "UNCLASSIFIED_FAILURE" {
				want = "STORAGE_GOVERNANCE_UNAVAILABLE"
			}
			require.Equal(t, want, dto.CodeOf(err))
			require.Equal(t, code == "STORAGE_CAPACITY_EXHAUSTED", errors.Is(err, ErrInvocationStorageCapacity))
			require.NotContains(t, err.Error(), private)
		})
	}
	a := NewAuditService(AuditServiceOptions{})
	a.SetStorageAdmission(func(context.Context) error { return fmt.Errorf("lookup: %s", "sensitive-database-error") })
	require.Equal(t, "STORAGE_GOVERNANCE_UNAVAILABLE", dto.CodeOf(a.checkStorageAdmission(context.Background())))
	a.SetStorageAdmission(func(context.Context) error { return nil })
	require.NoError(t, a.checkStorageAdmission(context.Background()))
}

func TestPublicAdmissionErrorOnlyPreservesSafeRecoveryCode(t *testing.T) {
	for _, cause := range []string{"DATABASE_RETENTION_RESTORE_CONFIRMATION_REQUIRED", "password=secret"} {
		err := dto.WithDetails(dto.NewErrorWithCode(503, "STORAGE_GOVERNANCE_UNAVAILABLE", "private", errors.New("private")), map[string]interface{}{"cause_code": cause, "sql": "private"})
		out := PublicStorageAdmissionError(fmt.Errorf("context: %w", err))
		require.NotContains(t, out.Error(), "private")
		require.NotContains(t, dto.DetailsOf(out), "sql")
		if cause[0] == 'D' {
			require.Equal(t, cause, dto.DetailsOf(out)["cause_code"])
		} else {
			require.Empty(t, dto.DetailsOf(out))
		}
	}
}
