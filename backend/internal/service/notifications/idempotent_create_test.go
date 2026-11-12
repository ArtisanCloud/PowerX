package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	host "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func creationFixture(t *testing.T) (*Service, context.Context, *setting.PluginInstanceConfig, IdempotentCreateInput) {
	t.Helper()
	db := host.Database(t)
	models := []any{&nm.Notification{}, &nm.CreationReceipt{}, &im.User{}, &im.Member{}}
	for _, model := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model))
		for _, f := range stmt.Schema.Fields {
			f.DefaultValue = strings.TrimSuffix(f.DefaultValue, "::jsonb")
		}
	}
	require.NoError(t, db.AutoMigrate(models...))
	require.NoError(t, db.AutoMigrate(&nm.CreationReceipt{}))
	tenant := uuid.NewString()
	ctx, credential := host.Actor(t, db, tenant, "plugin.notification.idempotency.test")
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: CreateCapabilityID, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: CreateCapabilityID, TenantUUID: tenant, Status: "published", ContractRef: "v1", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	host.Grant(t, db, credential, CreateCapabilityID)
	u := im.User{Email: "idempotent@fixture.test", Status: 1}
	require.NoError(t, db.Create(&u).Error)
	m := im.Member{TenantUUID: tenant, UserID: u.ID, Username: "recipient", Status: 1}
	require.NoError(t, db.Create(&m).Error)
	input := IdempotentCreateInput{IdempotencyKey: "crm.approval.wait24.fixture", MemberUUID: m.UUID.String(), Title: "fixture_title", Content: "fixture_content", Metadata: json.RawMessage(`{"value":9999999999999999.99,"nested":{"b":2,"a":1}}`), Navigation: &Navigation{Label: "fixture_action", Path: "/_p/plugin.crm/admin/approval"}}
	return NewService(db), ctx, credential, input
}

func TestIdempotentCreationReplaysOriginalAfterReadAndDeleteWithoutResurrection(t *testing.T) {
	s, ctx, _, input := creationFixture(t)
	first, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	id := first.Notification["id"].(string)
	require.NoError(t, s.MarkRead(ctx, firstTenant(s, t), input.MemberUUID, id))
	require.NoError(t, s.Delete(ctx, firstTenant(s, t), input.MemberUUID, id))
	input.Metadata = json.RawMessage(`{"nested":{"a":1,"b":2},"value":9999999999999999.99}`)
	replayed, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, id, replayed.Notification["id"])
	originalJSON, _ := json.Marshal(first.Notification)
	replayJSON, _ := json.Marshal(replayed.Notification)
	require.JSONEq(t, string(originalJSON), string(replayJSON))
	require.Contains(t, string(replayJSON), "9999999999999999.99")
	var notifications, receipts int64
	require.NoError(t, s.db.Model(&nm.Notification{}).Count(&notifications).Error)
	require.Zero(t, notifications)
	require.NoError(t, s.db.Model(&nm.CreationReceipt{}).Count(&receipts).Error)
	require.EqualValues(t, 1, receipts)
}

func TestIdempotentCreationCanonicalizesAbsentMetadataAndExplicitNull(t *testing.T) {
	s, ctx, _, input := creationFixture(t)
	input.Metadata = nil
	first, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	input.Metadata = json.RawMessage(`null`)
	again, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	require.True(t, again.Replayed)
	require.Equal(t, first.Notification["id"], again.Notification["id"])
	raw, _ := json.Marshal(again.Notification)
	require.Contains(t, string(raw), `"metadata":null`)
}

func firstTenant(s *Service, t *testing.T) string {
	t.Helper()
	var row nm.CreationReceipt
	require.NoError(t, s.db.First(&row).Error)
	return row.TenantUUID
}

func TestIdempotentCreationRejectsPayloadConflictAndSeparatesServiceOwners(t *testing.T) {
	s, ctx, _, input := creationFixture(t)
	first, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	for _, change := range []func(*IdempotentCreateInput){func(i *IdempotentCreateInput) { i.Content += "_changed" }, func(i *IdempotentCreateInput) { i.IsImportant = true }, func(i *IdempotentCreateInput) { i.Navigation = nil }, func(i *IdempotentCreateInput) { i.Metadata = json.RawMessage(`{"value":1}`) }} {
		changed := input
		change(&changed)
		_, err := s.CreateIdempotent(ctx, changed)
		var coded *dto.AppError
		require.ErrorAs(t, err, &coded)
		require.Equal(t, "NOTIFICATION_IDEMPOTENCY_CONFLICT", coded.Code)
	}
	other, cred := host.Actor(t, s.db, firstTenant(s, t), "plugin.notification.other")
	host.Grant(t, s.db, cred, CreateCapabilityID)
	second, err := s.CreateIdempotent(other, input)
	require.NoError(t, err)
	require.NotEqual(t, first.Notification["id"], second.Notification["id"])
}

func TestIdempotentCreationRevalidatesRevokedGrantAndDisabledMemberOnEveryReplay(t *testing.T) {
	s, ctx, cred, input := creationFixture(t)
	first, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	host.Grant(t, s.db, cred)
	_, err = s.CreateIdempotent(ctx, input)
	require.Error(t, err)
	host.Grant(t, s.db, cred, CreateCapabilityID)
	require.NoError(t, s.db.Model(&im.Member{}).Where("uuid = ?", input.MemberUUID).Update("status", 0).Error)
	_, err = s.CreateIdempotent(ctx, input)
	require.ErrorIs(t, err, ErrRecipientInvalid)
	require.NoError(t, s.db.Model(&im.Member{}).Where("uuid = ?", input.MemberUUID).Update("status", 1).Error)
	again, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.Notification["id"], again.Notification["id"])
}

func TestIdempotentCreationRollbackDoesNotConsumeKeyOrLeaveNotification(t *testing.T) {
	for _, phase := range []string{"notification", "receipt"} {
		t.Run(phase, func(t *testing.T) {
			s, ctx, _, input := creationFixture(t)
			abort := func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && (phase == "notification" && tx.Statement.Schema.Table == (nm.Notification{}).TableName() || phase == "receipt" && tx.Statement.Schema.Table == (nm.CreationReceipt{}).TableName()) {
					tx.AddError(errors.New("fixture_rollback"))
				}
			}
			if phase == "notification" {
				require.NoError(t, s.db.Callback().Create().Before("gorm:create").Register("fixture_abort", abort))
				defer s.db.Callback().Create().Remove("fixture_abort")
			} else {
				require.NoError(t, s.db.Callback().Update().Before("gorm:update").Register("fixture_abort", abort))
				defer s.db.Callback().Update().Remove("fixture_abort")
			}
			_, err := s.CreateIdempotent(ctx, input)
			require.ErrorContains(t, err, "fixture_rollback")
			var n int64
			require.NoError(t, s.db.Model(&nm.CreationReceipt{}).Count(&n).Error)
			require.Zero(t, n)
			require.NoError(t, s.db.Model(&nm.Notification{}).Count(&n).Error)
			require.Zero(t, n)
			s.db.Callback().Create().Remove("fixture_abort")
			s.db.Callback().Update().Remove("fixture_abort")
			_, err = s.CreateIdempotent(ctx, input)
			require.NoError(t, err)
		})
	}
}

func TestIdempotentCapabilityIsFixedTypedAndPublishesOnlyOnce(t *testing.T) {
	s, ctx, _, input := creationFixture(t)
	published := 0
	invoker := &CapabilityInvoker{Service: s, Publish: func(context.Context, string, any) { published++ }}
	raw, _ := json.Marshal(input)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	body["operation"] = "create_idempotent"
	in := cap.CoreCapabilityInvokeInput{CapabilityID: CreateCapabilityID, TenantUUID: reqctx.GetTenantUUID(ctx), Method: "INVOKE", Endpoint: CapabilityEndpoint, Body: body}
	for n := 0; n < 2; n++ {
		_, err := invoker.InvokeCoreCapability(ctx, in)
		require.NoError(t, err)
	}
	require.Equal(t, 1, published)
	for _, key := range []string{"endpoint", "tenant_uuid", "headers", "method"} {
		body[key] = "forbidden"
		_, err := invoker.InvokeCoreCapability(ctx, in)
		require.Error(t, err)
		delete(body, key)
	}
}

func TestIdempotentCreationPostgresConcurrentRetriesCommitOneReceipt(t *testing.T) {
	if os.Getenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN") == "" {
		t.Skip("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN required")
	}
	s, ctx, _, input := creationFixture(t)
	const count = 10
	var wg sync.WaitGroup
	ids := make(chan string, count)
	failures := make(chan error, count)
	for n := 0; n < count; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.CreateIdempotent(ctx, input)
			if err != nil {
				failures <- err
			} else {
				ids <- r.Notification["id"].(string)
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	unique := map[string]bool{}
	for id := range ids {
		unique[id] = true
	}
	require.Len(t, unique, 1)
	var n int64
	require.NoError(t, s.db.Model(&nm.CreationReceipt{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, s.db.Model(&nm.Notification{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
}

func TestIdempotentCreationRejectsInvalidKeysBroadcastAndMissingSchemaWithoutWrites(t *testing.T) {
	s, ctx, _, input := creationFixture(t)
	for _, key := range []string{"", "has space", "non_ascii_中", strings.Repeat("k", 257)} {
		bad := input
		bad.IdempotencyKey = key
		_, err := s.CreateIdempotent(ctx, bad)
		require.Error(t, err)
	}
	for _, member := range []string{"", uuid.Nil.String(), "bad", strings.ToUpper(input.MemberUUID)} {
		if member == input.MemberUUID {
			continue
		}
		bad := input
		bad.MemberUUID = member
		_, err := s.CreateIdempotent(ctx, bad)
		require.Error(t, err)
	}
	require.NoError(t, s.db.Migrator().DropTable(&nm.CreationReceipt{}))
	_, err := s.CreateIdempotent(ctx, input)
	require.Error(t, err)
	var n int64
	require.NoError(t, s.db.Model(&nm.Notification{}).Count(&n).Error)
	require.Zero(t, n)
}
