package agent_session

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPostgresMigrationAndConcurrentIdempotency(t *testing.T) {
	dsn := os.Getenv("POWERX_SESSION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POWERX_SESSION_TEST_POSTGRES_DSN_not_set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	schema := "px_session_contract_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	old := coremodel.PowerXSchema
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		// schema is generated locally, never taken from a supplied config/path.
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		coremodel.PowerXSchema = old
		require.NoError(t, sqlDB.Close())
	})
	for i := 0; i < 3; i++ {
		require.NoError(t, db.AutoMigrate(&m.ServiceSession{}, &m.ServiceMessage{}, &m.ServiceInvocation{}, &capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &setting.PluginCapabilityApproval{}))
	}
	require.NoError(t, db.Exec(`CREATE TABLE "`+schema+`".agents (id bigserial primary key, uuid uuid NOT NULL UNIQUE, tenant_uuid uuid NOT NULL, owner_plugin_id text, status text, deleted_at timestamptz)`).Error)
	tenant, agent := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO "`+schema+`".agents (uuid,tenant_uuid,owner_plugin_id,status) VALUES (?,?,?,?)`, agent, tenant, "plugin.test", "active").Error)
	for _, capability := range []string{SessionCapability, InvokeCapability} {
		require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: capability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
		require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tenant.String(), CapabilityID: capability, Status: "published", ContractRef: "test", Version: 1}).Error)
	}
	raw, _ := json.Marshal(map[string]any{"client_id": "plugin.test", "allowed_capabilities": []string{SessionCapability, InvokeCapability}})
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant.String(), PluginID: "plugin.test", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(raw)}).Error)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant.String()), &reqctx.CoreXClaims{TenantUUID: tenant.String(), PluginID: "plugin.test", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:plugin.test", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	executor := &controlledExecutor{started: make(chan struct{}), release: make(chan struct{})}
	svc := NewServiceWithExecutor(db, executor)
	session, err := svc.Create(ctx, agent, "")
	require.NoError(t, err)
	const parallel = 12
	var wg sync.WaitGroup
	results := make([]Message, parallel)
	errs := make([]error, parallel)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.Append(ctx, session.SessionUUID, "same-key", "user", "test")
		}(i)
	}
	wg.Wait()
	for i := range results {
		require.NoError(t, errs[i])
		require.Equal(t, results[0].MessageUUID, results[i].MessageUUID)
	}
	runs := make([]Invocation, parallel)
	for i := range runs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runs[i], errs[i] = svc.Invoke(ctx, session.SessionUUID, results[0].MessageUUID, "same-run")
		}(i)
	}
	wg.Wait()
	for i := range runs {
		require.NoError(t, errs[i])
		require.Equal(t, runs[0].InvocationUUID, runs[i].InvocationUUID)
	}
	select {
	case <-executor.started:
	case <-time.After(2 * time.Second):
		t.Fatal("executor_start_timeout")
	}
	close(executor.release)
	require.Eventually(t, func() bool {
		run, err := svc.GetInvocation(ctx, session.SessionUUID, runs[0].InvocationUUID)
		return err == nil && run.Status == "succeeded"
	}, 3*time.Second, 20*time.Millisecond)
	_, total, err := svc.Messages(ctx, session.SessionUUID, 1, 100)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	// Repeat migrations after real data exists; UUIDs and history must survive.
	require.NoError(t, db.AutoMigrate(&m.ServiceSession{}, &m.ServiceMessage{}, &m.ServiceInvocation{}, &setting.PluginCapabilityApproval{}))
	_, err = svc.Get(ctx, session.SessionUUID)
	require.NoError(t, err)
}
