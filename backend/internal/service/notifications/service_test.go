package notifications

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateValidatesRecipientAndKeepsHistoryMemberScoped(t *testing.T) {
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/notifications.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	models := []any{&tm.Tenant{}, &im.User{}, &im.Member{}, &nm.Notification{}}
	for _, m := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(m))
		for _, f := range stmt.Schema.Fields {
			f.DefaultValue = strings.TrimSuffix(f.DefaultValue, "::jsonb")
		}
	}
	require.NoError(t, db.AutoMigrate(models...))
	tenant, other := uuid.NewString(), uuid.NewString()
	for _, id := range []string{tenant, other} {
		require.NoError(t, db.Create(&tm.Tenant{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(id)}, Key: id, Name: id, Status: 1}).Error)
	}
	u := im.User{Email: "b@example.test", Status: 1}
	require.NoError(t, db.Create(&u).Error)
	b := im.Member{TenantUUID: tenant, UserID: u.ID, Username: "b", Status: 1}
	require.NoError(t, db.Create(&b).Error)
	s := NewService(db)
	ctx := context.Background()
	input := CreateInput{TenantUUID: tenant, MemberUUID: b.UUID.String(), Title: "personal", Content: "safe reminder"}
	item, err := s.Create(ctx, input)
	require.NoError(t, err)
	require.Equal(t, b.UUID.String(), item.MemberUUID)
	for _, target := range []string{"invalid", uuid.NewString()} {
		bad := input
		bad.MemberUUID = target
		_, err = s.Create(ctx, bad)
		require.ErrorIs(t, err, ErrRecipientInvalid)
	}
	bad := input
	bad.TenantUUID = other
	_, err = s.Create(ctx, bad)
	require.ErrorIs(t, err, ErrRecipientInvalid)
	require.NoError(t, db.Model(&b).Update("status", 0).Error)
	_, err = s.Create(ctx, input)
	require.ErrorIs(t, err, ErrRecipientInvalid)
	require.NoError(t, db.Model(&b).Update("status", 1).Error)
	items, total, err := s.List(ctx, ListInput{TenantUUID: tenant, MemberUUID: uuid.NewString()})
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, items)
	items, total, err = s.List(ctx, ListInput{TenantUUID: tenant, MemberUUID: b.UUID.String()})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	input.MemberUUID = ""
	_, err = s.Create(ctx, input)
	require.NoError(t, err)
}

func TestNotificationTargetNeverFallsBackFromMalformedMember(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"userId":null}`, `{"userId":42}`, `{"userId":"invalid"}`, `{"userId":"` + uuid.NewString() + `","member_uuid":"` + uuid.NewString() + `"}`} {
		_, err := Target([]byte(raw))
		require.Error(t, err, raw)
	}
	for _, payload := range []map[string]any{{"title": "broadcast"}, {"userId": ""}} {
		raw, _ := json.Marshal(payload)
		target, err := Target(raw)
		require.NoError(t, err)
		require.Empty(t, target)
	}
}
