package ops

import (
	"context"
	platformopsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/platform_ops/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestBackupControlLegacyGRPCWritesFailExplicitly(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	h := NewDeployHandler(&shared.Deps{DB: db})
	_, e = h.UpsertBackupPolicy(ctx, &platformopsv1.UpsertBackupPolicyRequest{Policy: &platformopsv1.BackupPolicy{Name: "legacy"}})
	require.Equal(t, codes.FailedPrecondition, status.Code(e))
	require.Equal(t, "BACKUP_POLICY_CONTROL_REQUIRED", status.Convert(e).Message())
	require.Equal(t, codes.Unavailable, status.Code(mapBackupError(dto.NewErrorWithCode(503, "BACKUP_POLICY_CONTROL_UNAVAILABLE", "unavailable", nil))))
}
