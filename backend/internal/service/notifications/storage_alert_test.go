package notifications

import (
	"context"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"strings"
	"testing"
)

func TestStorageNotificationRetriesDoNotDuplicateOrResurrectDeletedHistory(t *testing.T) {
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/storage-notifications.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	models := []any{&tm.Tenant{}, &im.User{}, &im.Member{}, &nm.Notification{}}
	for _, m := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(m))
		for _, f := range stmt.Schema.Fields {
			f.DefaultValue = strings.TrimSuffix(f.DefaultValue, "::jsonb")
		}
	}
	require.NoError(t, db.AutoMigrate(models...))
	tenant := uuid.NewString()
	require.NoError(t, db.Create(&tm.Tenant{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(tenant)}, Key: tenant, Name: tenant, Status: 1}).Error)
	u := im.User{Email: "storage@example.test", Status: 1}
	require.NoError(t, db.Create(&u).Error)
	member := im.Member{TenantUUID: tenant, UserID: u.ID, Username: "storage", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	s := NewService(db)
	ctx := context.Background()
	input := CreateInput{TenantUUID: tenant, MemberUUID: member.UUID.String(), Title: "Capacity alert", RelatedType: "storage_capacity", RelatedID: "stable-request"}
	first, err := s.CreateStorageAlertOnce(ctx, input)
	require.NoError(t, err)
	second, err := s.CreateStorageAlertOnce(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.UUID, second.UUID)
	require.NoError(t, s.Delete(ctx, tenant, member.UUID.String(), first.UUID.String()))
	deleted, err := s.CreateStorageAlertOnce(ctx, input)
	require.NoError(t, err)
	require.True(t, deleted.DeletedAt.Valid)
	rows, count, err := s.List(ctx, ListInput{TenantUUID: tenant, MemberUUID: member.UUID.String()})
	require.NoError(t, err)
	require.Zero(t, count)
	require.Empty(t, rows)
	input.MemberUUID = uuid.NewString()
	_, err = s.CreateStorageAlertOnce(ctx, input)
	require.ErrorIs(t, err, ErrRecipientInvalid)
}
