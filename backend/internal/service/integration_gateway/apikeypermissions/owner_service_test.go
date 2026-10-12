package apikeypermissions

import (
	"context"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"sync"
	"testing"

	"fmt"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net"
	"net/url"
)

const ownerTestTenant = "6b5d0240-9920-46da-b707-88200e0f51ea"

func ownerFixture(t *testing.T) (*gorm.DB, context.Context, *iammodel.APIKeyProfile, *gwmodel.IntegrationGatewayAPIKey, []uint64) {
	t.Helper()
	old := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = old })
	var db *gorm.DB
	var err error
	if path := os.Getenv("POWERX_OWNER_POSTGRES_CONFIG"); path != "" {
		raw, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var cfg struct {
			Database struct {
				Host     string `yaml:"host"`
				Port     int    `yaml:"port"`
				Username string `yaml:"username"`
				User     string `yaml:"user"`
				Password string `yaml:"password"`
				Database string `yaml:"database"`
				DBName   string `yaml:"dbname"`
				SSLMode  string `yaml:"sslmode"`
			} `yaml:"database"`
		}
		require.NoError(t, yaml.Unmarshal(raw, &cfg))
		d := cfg.Database
		if d.Username == "" {
			d.Username = d.User
		}
		if d.Database == "" {
			d.Database = d.DBName
		}
		if d.SSLMode == "" {
			d.SSLMode = "disable"
		}
		address := url.URL{Scheme: "postgres", User: url.UserPassword(d.Username, d.Password), Host: net.JoinHostPort(d.Host, fmt.Sprint(d.Port)), Path: d.Database, RawQuery: "sslmode=" + d.SSLMode}
		db, err = gorm.Open(postgres.Open(address.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)

		db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		model.PowerXSchema = "owner_accept_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		require.NoError(t, db.Exec(`CREATE SCHEMA "`+model.PowerXSchema+`"`).Error)
	} else {
		db, err = gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	}
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// PostgreSQL cleanup above must drop its isolated schema before closing.
	if os.Getenv("POWERX_OWNER_POSTGRES_CONFIG") == "" {
		t.Cleanup(func() { _ = sqlDB.Close() })
	} else {
		t.Cleanup(func() {
			require.NoError(t, db.Exec(`DROP SCHEMA IF EXISTS "`+model.PowerXSchema+`" CASCADE`).Error)
			_ = sqlDB.Close()
		})
	}
	require.NoError(t, db.AutoMigrate(&iammodel.APIKeyProfile{}, &iammodel.APIKeyProfilePermission{}, &iammodel.Permission{}, &gwmodel.IntegrationGatewayAPIKey{}, &gwmodel.IntegrationGatewayAPIKeyPermission{}, &gwmodel.IntegrationGatewayAPIKeyAuditLog{}))
	ctx := reqctx.WithTenantUUID(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, TenantUUID: ownerTestTenant}), ownerTestTenant)
	profile := &iammodel.APIKeyProfile{TenantUUID: ownerTestTenant, Key: "owner-fixture", Name: "owner fixture", Status: 1}
	require.NoError(t, db.Create(profile).Error)
	ids := []uint64{}
	for _, action := range []string{"read", "delete"} {
		meta, _ := json.Marshal(map[string]any{"api_key": map[string]any{"scope": "_scope.scheduler.jobs.service_" + action, "action": action, "resource_type": "api", "resource_pattern": "scheduler_jobs", "effect": "allow"}})
		permission := &iammodel.Permission{Module: "scheduler", Resource: "jobs.service", Action: action, Status: iammodel.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(meta)}
		require.NoError(t, db.Create(permission).Error)
		ids = append(ids, permission.ID)
	}
	require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(db).GrantByIDsTx(db, profile.ID, ids))
	key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).Create(ctx, &gwmodel.IntegrationGatewayAPIKey{TenantUUID: ownerTestTenant, ProfileID: profile.ID, Name: "legacy-key", KeyPrefix: "owner-fixture", KeyHash: uuid.NewString(), Status: "active"})
	require.NoError(t, err)
	svc := NewAPIKeyOwnerService(db)
	base, err := svc.ProfileGrants(ctx, db, profile.ID)
	require.NoError(t, err)
	rows, err := BindPluginOwners(key.UUID, base, []string{"com.powerx.plugins.scrm"})
	require.NoError(t, err)
	require.NoError(t, gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ReplaceAll(ctx, key.UUID, rows))
	return db, ctx, profile, key, ids
}

func ownerAllows(t *testing.T, db *gorm.DB, key uuid.UUID, owner string) bool {
	t.Helper()
	rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(context.Background(), key)
	require.NoError(t, err)
	owned := []gwmodel.IntegrationGatewayAPIKeyPermission{}
	for _, r := range rows {
		if r.PluginID == owner {
			owned = append(owned, r)
		}
	}
	return gwrepo.APIKeyPermissionGranted(owned, "_scope.scheduler.jobs.service_delete", "delete", "api", "scheduler_jobs")
}

func TestOwnerBindingSurvivesProfileClearAndRestore(t *testing.T) {
	db, ctx, profile, key, ids := ownerFixture(t)
	svc := NewAPIKeyOwnerService(db)
	// Legacy bindings survive the same reconciliation used by both Profile save paths.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
	result, err := svc.SetOwners(ctx, ownerTestTenant, key.UUID, []string{"com.powerx.plugins.crm"}, "root")
	require.NoError(t, err)
	require.Equal(t, []string{"com.powerx.plugins.crm"}, result.PluginIDs)
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
	require.False(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).RevokeByIDsTx(tx, profile.ID, ids); err != nil {
			return err
		}
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	result, err = svc.GetOwners(ctx, ownerTestTenant, key.UUID)
	require.NoError(t, err)
	require.Equal(t, []string{"com.powerx.plugins.crm"}, result.PluginIDs)
	require.False(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).GrantByIDsTx(tx, profile.ID, ids); err != nil {
			return err
		}
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
	_, err = svc.SetOwners(ctx, ownerTestTenant, key.UUID, []string{}, "root")
	require.NoError(t, err)
	require.NoError(t, syncActiveAPIKeyPermissions(ctx, db, ownerTestTenant, profile.ID, ids))
	require.False(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
	result, err = svc.GetOwners(ctx, ownerTestTenant, key.UUID)
	require.NoError(t, err)
	require.Empty(t, result.PluginIDs)
}

func TestOwnerBindingValidationIsolationAndAudit(t *testing.T) {
	db, ctx, _, key, _ := ownerFixture(t)
	svc := NewAPIKeyOwnerService(db)
	for _, ids := range [][]string{{"*"}, {"com.powerx.*"}, {""}, {"../crm"}, {"com.PowerX.crm"}} {
		_, err := svc.SetOwners(ctx, ownerTestTenant, key.UUID, ids, "root")
		require.Equal(t, 400, dto.StatusCode(err))
		require.Equal(t, "API_KEY_PLUGIN_OWNER_INVALID", dto.CodeOf(err))
	}
	foreignTenant := uuid.NewString()
	foreignCtx := reqctx.WithTenantUUID(ctx, foreignTenant)
	_, err := svc.SetOwners(foreignCtx, foreignTenant, key.UUID, []string{"com.powerx.plugins.crm"}, "root")
	require.Equal(t, 404, dto.StatusCode(err))
	nonAdmin := reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 2, TenantUUID: ownerTestTenant, Roles: []string{"role_member"}})
	_, err = svc.SetOwners(nonAdmin, ownerTestTenant, key.UUID, []string{"com.powerx.plugins.crm"}, "member")
	require.Equal(t, 403, dto.StatusCode(err))
	apiKeyCtx := reqctx.WithAuthenticatedAPIKeyHash(ctx, "authenticated-fixture")
	_, err = svc.SetOwners(apiKeyCtx, ownerTestTenant, key.UUID, []string{"com.powerx.plugins.crm"}, "apikey")
	require.Equal(t, 403, dto.StatusCode(err))
	result, err := svc.SetOwners(ctx, ownerTestTenant, key.UUID, []string{"com.powerx.plugins.crm", "com.powerx.plugins.scrm", "com.powerx.plugins.crm"}, "root")
	require.NoError(t, err)
	require.Len(t, result.PluginIDs, 2)
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
	require.False(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.other"))
	var auditCount int64
	require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyAuditLog{}).Count(&auditCount).Error)
	require.EqualValues(t, 1, auditCount)
}

func TestConfiguredOwnersAreNeverInferredAgain(t *testing.T) {
	rows := []gwmodel.IntegrationGatewayAPIKeyPermission{{PluginID: "com.powerx.plugins.scrm", Effect: "allow", Scope: "scope", Action: "read", ResourceType: "api", ResourcePattern: "jobs"}, {PluginID: "*"}}
	ids, err := EffectivePluginOwners(nil, rows)
	require.NoError(t, err)
	require.Equal(t, []string{"com.powerx.plugins.scrm"}, ids)
	ids, err = EffectivePluginOwners(EncodePluginOwners([]string{}), rows)
	require.NoError(t, err)
	require.Empty(t, ids)
	_, err = EffectivePluginOwners(datatypes.JSON(`{"plugin_ids":"*"}`), rows)
	require.Error(t, err)
}

func TestLegacyOwnerScopesAreNotExpandedByProfileSave(t *testing.T) {
	db, ctx, profile, key, ids := ownerFixture(t)
	svc := NewAPIKeyOwnerService(db)
	require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyPermission{}).Where("api_key_uuid = ? AND action = ?", key.UUID, "delete").Update("plugin_id", "com.powerx.plugins.crm").Error)
	verify := func() {
		rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(ctx, key.UUID)
		require.NoError(t, err)
		for _, r := range rows {
			if r.Action == "read" {
				require.Equal(t, "com.powerx.plugins.scrm", r.PluginID)
			} else {
				require.Equal(t, "com.powerx.plugins.crm", r.PluginID)
			}
		}
		require.Len(t, rows, 2)
	}
	require.NoError(t, syncActiveAPIKeyPermissions(ctx, db, ownerTestTenant, profile.ID, ids))
	verify()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).RevokeByIDsTx(tx, profile.ID, ids); err != nil {
			return err
		}
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).GrantByIDsTx(tx, profile.ID, ids); err != nil {
			return err
		}
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	verify()
}

func TestOwnerPostgresConcurrentProfileSaveAndOwnerChange(t *testing.T) {
	if os.Getenv("POWERX_OWNER_POSTGRES_CONFIG") == "" {
		t.Skip("opt-in PostgreSQL row-lock acceptance")
	}
	db, ctx, profile, key, _ := ownerFixture(t)
	svc := NewAPIKeyOwnerService(db)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_, err := svc.SetOwners(ctx, ownerTestTenant, key.UUID, []string{"com.powerx.plugins.crm"}, "root")
			if err != nil {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			err := db.Transaction(func(tx *gorm.DB) error {
				_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
				return err
			})
			if err != nil {
				errs <- err
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	result, err := svc.GetOwners(ctx, ownerTestTenant, key.UUID)
	require.NoError(t, err)
	require.Equal(t, []string{"com.powerx.plugins.crm"}, result.PluginIDs)
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
	require.False(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
}
