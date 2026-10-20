package backup_ops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	notifications "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	ops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func notificationFixture(t *testing.T) (*gorm.DB, string, map[string]im.Member) {
	t.Helper()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/notifications.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	models := []any{&tm.Tenant{}, &im.User{}, &im.Member{}, &nm.Notification{}}
	for _, model := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model))
		for _, field := range stmt.Schema.Fields {
			field.DefaultValue = strings.TrimSuffix(field.DefaultValue, "::jsonb")
		}
	}
	require.NoError(t, db.AutoMigrate(models...))
	tenant, other := uuid.NewString(), uuid.NewString()
	for _, id := range []string{tenant, other} {
		require.NoError(t, db.Create(&tm.Tenant{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(id)}, Key: id, Name: id, Status: 1}).Error)
	}
	members := map[string]im.Member{}
	for _, name := range []string{"operator", "root", "other-root", "disabled-root", "disabled-user"} {
		user := im.User{Email: name + "@backup.test", IsRoot: name != "operator", Status: 1}
		if name == "disabled-user" {
			user.Status = 0
		}
		require.NoError(t, db.Create(&user).Error)
		if name == "disabled-user" {
			require.NoError(t, db.Model(&user).Update("status", 0).Error)
		}
		m := im.Member{TenantUUID: tenant, UserID: user.ID, Username: name, Status: 1}
		if name == "other-root" {
			m.TenantUUID = other
		}
		if name == "disabled-root" {
			m.Status = 0
		}
		require.NoError(t, db.Create(&m).Error)
		if name == "disabled-root" {
			require.NoError(t, db.Model(&m).Update("status", 0).Error)
		}
		members[name] = m
	}
	return db, tenant, members
}

func TestTaskResultsPersistMemberNotificationsAndSafeTaskNavigation(t *testing.T) {
	db, tenant, members := notificationFixture(t)
	ctx := reqctx.WithMemberUUID(reqctx.WithTenantUUID(context.Background(), tenant), members["operator"].UUID.String())
	for _, status := range []ops.BackupJobStatus{ops.BackupJobStatusSuccess, ops.BackupJobStatusFailed, ops.BackupJobStatusRunning} {
		job := &ops.BackupJob{Status: status, ErrorMessage: "secret server connection string"}
		job.ID = 801
		publishBackupJobStatus(ctx, db, job)
	}
	drill := &ops.RestoreDrillRecord{SourceJobID: 801, Status: ops.RestoreDrillStatusFailed, ReportURI: "private database path"}
	drill.ID = 99
	publishRestoreDrillStatus(ctx, db, drill)
	svc := notifications.NewService(db)
	items, total, err := svc.List(ctx, notifications.ListInput{TenantUUID: tenant, MemberUUID: members["operator"].UUID.String()})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	for i := range items {
		item := &items[i]
		found, err := svc.Get(ctx, tenant, item.MemberUUID, item.UUID.String())
		require.NoError(t, err)
		require.NotNil(t, found)
		require.NotContains(t, item.Content, "secret")
		require.NotContains(t, item.Content, "private")
		path := "/ops/backup?job_id=801"
		if item.RelatedType == "restore_drill" {
			path = "/ops/backup?drill_id=99"
		}
		require.Contains(t, string(item.Actions), path)
		require.Equal(t, item.Type == "error", item.IsImportant)
		_, otherTotal, err := svc.List(ctx, notifications.ListInput{TenantUUID: tenant, MemberUUID: members["root"].UUID.String()})
		require.NoError(t, err)
		require.Zero(t, otherTotal)
	}
}

func TestScheduledTaskRecipientsNeverFallBackToTenantBroadcast(t *testing.T) {
	db, tenant, members := notificationFixture(t)
	ctx := reqctx.WithTenantUUID(context.Background(), tenant)
	job := &ops.BackupJob{Status: ops.BackupJobStatusSuccess, TriggerType: ops.BackupTriggerTypeScheduled}
	job.ID = 802
	publishBackupJobStatus(ctx, db, job)
	var items []nm.Notification
	require.NoError(t, db.Find(&items).Error)
	require.Len(t, items, 1)
	require.Equal(t, members["root"].UUID.String(), items[0].MemberUUID)
	// An explicit inactive/cross-tenant/malformed identity must not fall back to Root.
	for _, target := range []string{members["disabled-root"].UUID.String(), members["other-root"].UUID.String(), members["disabled-user"].UUID.String(), "invalid"} {
		publishBackupJobStatus(reqctx.WithMemberUUID(ctx, target), db, job)
	}
	job.TriggerType = ops.BackupTriggerTypeManual
	publishBackupJobStatus(ctx, db, job)                  // No member on a manual request.
	publishBackupJobStatus(context.Background(), db, job) // No tenant.
	var total int64
	require.NoError(t, db.Model(&nm.Notification{}).Count(&total).Error)
	require.EqualValues(t, 1, total)
}

func TestTaskNotificationResolvesNumericJWTIdentityLikeTheReadAPI(t *testing.T) {
	db, tenant, members := notificationFixture(t)
	member := members["operator"]
	claims := &reqctx.CoreXClaims{TenantUUID: tenant, MemberID: member.ID, UserID: member.UserID}
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), claims)
	ids, err := taskNotificationRecipients(ctx, db, tenant, false)
	require.NoError(t, err)
	require.Equal(t, []string{member.UUID.String()}, ids)
	// A mismatched numeric member/user must never resolve to somebody else's UUID.
	claims.UserID = members["root"].UserID
	ids, err = taskNotificationRecipients(ctx, db, tenant, true)
	require.ErrorIs(t, err, notifications.ErrRecipientInvalid)
	require.Empty(t, ids)
}

type backupTopicLookup struct{}

func (backupTopicLookup) FindByComposite(context.Context, string, string, string) (*ef.TopicDefinition, error) {
	return &ef.TopicDefinition{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.New()}, TenantKey: "global", Namespace: "_topic.system", Name: "notification", Status: 1, Lifecycle: ef.TopicLifecycleActive}, nil
}

type backupACL struct{}

func (backupACL) HasPermission(context.Context, string, uuid.UUID, string, string, time.Time) (bool, error) {
	return false, nil
}

func TestTaskNotificationWebSocketUsesPersistedIDAndDoesNotPublishFailedWrites(t *testing.T) {
	db, tenant, members := notificationFixture(t)
	member := members["operator"]
	oldHub := wsbus.DefaultHub
	wsbus.DefaultHub = wsbus.NewHub()
	t.Cleanup(func() { wsbus.DefaultHub = oldHub })
	handler := wsbus.NewHandlerWithOptions(wsbus.HandlerOptions{DB: db, TopicLookup: backupTopicLookup{}, ACLStore: backupACL{}})
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{TenantUUID: tenant, MemberUUID: member.UUID.String(), MemberID: member.ID, UserID: member.UserID})
	ctx = reqctx.WithTenantUUID(ctx, tenant)
	engine := gin.New()
	engine.GET("/ws", func(c *gin.Context) { c.Request = c.Request.WithContext(ctx); handler.ServeWS(c) })
	server := httptest.NewServer(engine)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	var envelope dto.WSBusEnvelope
	require.NoError(t, conn.ReadJSON(&envelope))
	require.NoError(t, conn.WriteJSON(dto.WSBusCommand{Type: dto.WSBusCmdSubscribe, Topic: eventbus.TopicSystemNotification}))
	require.NoError(t, conn.ReadJSON(&envelope))
	require.Equal(t, dto.WSBusTypeAck, envelope.Type)
	job := &ops.BackupJob{Status: ops.BackupJobStatusSuccess}
	job.ID = 803
	publishBackupJobStatus(ctx, db, job)
	require.NoError(t, conn.ReadJSON(&envelope))
	var event struct {
		ID      string `json:"id"`
		Member  string `json:"userId"`
		Actions []any  `json:"actions"`
	}
	require.NoError(t, json.Unmarshal(envelope.Payload, &event))
	require.Equal(t, member.UUID.String(), event.Member)
	require.NotEmpty(t, event.Actions)
	item, err := notifications.NewService(db).Get(ctx, tenant, member.UUID.String(), event.ID)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, "803", item.RelatedID)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:notification-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "system_notifications" {
			tx.AddError(errors.New("notification storage unavailable"))
		}
	}))
	publishBackupJobStatus(ctx, db, job)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	require.Error(t, conn.ReadJSON(&envelope), "failed persistence must not emit a phantom notification")
}
