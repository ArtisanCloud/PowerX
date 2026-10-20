package authorizationbaseline

import (
	"errors"
	"testing"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func bootstrapDB(t *testing.T) *gorm.DB {
	t.Helper()
	old := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&im.AuthorizationTracking{}, &im.AuthorizationStream{}, &im.AuthorizationSegment{}, &im.User{}, &im.Member{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &tm.Tenant{}, &sm.PluginInstanceConfig{}))
	return db
}

func bootstrapPermission(t *testing.T, db *gorm.DB, plugin, resource string) im.Permission {
	t.Helper()
	p := im.Permission{Module: "work", Resource: resource, Action: "read", Effect: "allow", Status: im.PermissionStatusActive, Source: "plugin:" + plugin, Meta: datatypes.JSON(`{"authorization_baseline":true,"data_scope":"TENANT"}`)}
	require.NoError(t, db.Create(&p).Error)
	return p
}

func bootstrapTenant(t *testing.T, db *gorm.DB, plugin string, permission im.Permission, grant bool) fixture {
	t.Helper()
	f := fixture{db: db, tenant: uuid.NewString(), plugin: plugin, permission: permission}
	tenant := tm.Tenant{Key: uuid.NewString(), Status: tm.TenantStatusActive}
	tenant.UUID = uuid.MustParse(f.tenant)
	require.NoError(t, db.Create(&tenant).Error)
	user := im.User{Email: uuid.NewString() + "@example.invalid", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	f.member = im.Member{TenantUUID: f.tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "member", Status: 1}
	require.NoError(t, db.Create(&f.member).Error)
	require.NoError(t, db.Create(&sm.PluginInstanceConfig{TenantUUID: f.tenant, PluginID: plugin, Key: "auth.credentials", Enabled: true}).Error)
	if grant {
		f.role = im.Role{TenantUUID: f.tenant, Scope: "tenant", Code: "reader"}
		require.NoError(t, db.Create(&f.role).Error)
		require.NoError(t, db.Create(&im.RolePermission{RoleID: f.role.ID, RoleUUID: f.role.UUID, PermissionID: permission.ID, PermissionUUID: permission.PermissionUUID}).Error)
		f.binding = im.RoleBinding{TenantUUID: f.tenant, RoleID: f.role.ID, RoleUUID: f.role.UUID.String(), SubjectType: im.SubMember, SubjectID: f.member.ID, SubjectUUID: f.member.UUID.String(), DataScope: im.ScopeTenant}
		require.NoError(t, db.Create(&f.binding).Error)
	}
	return f
}

func trackingFor(t *testing.T, f fixture, code string) im.AuthorizationTracking {
	t.Helper()
	var row im.AuthorizationTracking
	require.NoError(t, f.db.Where("tenant_uuid=? AND plugin_id=? AND permission_code=?", f.tenant, f.plugin, code).Take(&row).Error)
	return row
}

func TestBootstrapAllApplicableTenantsPreservesExistingFacts(t *testing.T) {
	db := bootstrapDB(t)
	p := bootstrapPermission(t, db, "com.example.orders", "item")
	first := bootstrapTenant(t, db, "com.example.orders", p, true)
	second := bootstrapTenant(t, db, "com.example.orders", p, false)
	otherPermission := bootstrapPermission(t, db, "com.example.inventory", "stock")
	other := bootstrapTenant(t, db, "com.example.inventory", otherPermission, true)
	disabled := bootstrapTenant(t, db, first.plugin, p, false)
	require.NoError(t, db.Model(&tm.Tenant{}).Where("uuid=?", disabled.tenant).Update("status", 2).Error)
	disabledPlugin := bootstrapTenant(t, db, first.plugin, p, false)
	require.NoError(t, db.Model(&sm.PluginInstanceConfig{}).Where("tenant_uuid=?", disabledPlugin.tenant).Update("enabled", false).Error)
	for i, meta := range []string{`{}`, `{"authorization_baseline":"true"}`, `{"authorization_baseline":true,"type":"api"}`} {
		ignored := im.Permission{Module: "ignored", Resource: string(rune('a' + i)), Action: "read", Source: "plugin:" + first.plugin, Status: im.PermissionStatusActive, Meta: datatypes.JSON(meta)}
		require.NoError(t, db.Create(&ignored).Error)
	}
	inactive := bootstrapPermission(t, db, first.plugin, "inactive")
	require.NoError(t, db.Model(&inactive).Update("status", "deprecated").Error)
	// A trusted historical start and its decision must survive startup unchanged.
	at := Micro(time.Now().Add(-time.Hour))
	legacy := im.AuthorizationTracking{TenantUUID: first.tenant, PluginID: first.plugin, PermissionCode: "work.item:read", Status: "ready", KnownSince: &at, InitializationGuardUUID: uuid.NewString(), InitializedBy: "legacy-operator"}
	require.NoError(t, db.Create(&legacy).Error)
	legacy = trackingFor(t, first, "work.item:read")
	require.NoError(t, Reconcile(db, at))
	beforeStream, beforeDecision := first.current(t)
	require.NoError(t, Install(db))
	before := Micro(time.Now())
	require.NoError(t, EnsureInitialized(db))
	after := Micro(time.Now())
	var tracks []im.AuthorizationTracking
	require.NoError(t, db.Find(&tracks).Error)
	require.Len(t, tracks, 3)
	require.Equal(t, legacy, trackingFor(t, first, "work.item:read"))
	newTrack := trackingFor(t, second, "work.item:read")
	require.Equal(t, AutomaticInitializer, newTrack.InitializedBy)
	require.NotEqual(t, uuid.Nil, uuid.MustParse(newTrack.InitializationGuardUUID))
	require.False(t, newTrack.KnownSince.Before(before))
	require.False(t, newTrack.KnownSince.After(after))
	_, denied := second.current(t)
	require.Equal(t, Denied, denied.State, "initialization must not grant access")
	_, allowed := other.current(t)
	require.Equal(t, Allowed, allowed.State)
	require.Equal(t, "initialized", allowed.BaselineKind)
	require.Nil(t, allowed.GrantedAt, "unknown past is not a real grant timestamp")
	require.NoError(t, EnsureInitialized(db))
	afterStream, afterDecision := first.current(t)
	require.Equal(t, beforeStream, afterStream)
	require.Equal(t, beforeDecision, afterDecision)
	require.Equal(t, newTrack, trackingFor(t, second, "work.item:read"))
	var grants int64
	require.NoError(t, db.Model(&im.RoleBinding{}).Count(&grants).Error)
	require.EqualValues(t, 2, grants)
	// Previously inactive tenants and plugins get tracking when activated, without startup.
	require.NoError(t, db.Model(&tm.Tenant{}).Where("uuid=?", disabled.tenant).Update("status", tm.TenantStatusActive).Error)
	require.Equal(t, AutomaticInitializer, trackingFor(t, disabled, "work.item:read").InitializedBy)
	require.NoError(t, db.Model(&sm.PluginInstanceConfig{}).Where("tenant_uuid=?", disabledPlugin.tenant).Update("enabled", true).Error)
	require.Equal(t, AutomaticInitializer, trackingFor(t, disabledPlugin, "work.item:read").InitializedBy)
}

func TestAutomaticTrackingOnTenantPluginAndPermissionTransactions(t *testing.T) {
	for _, registrationOrder := range []string{"permission-first", "plugin-first"} {
		t.Run(registrationOrder, func(t *testing.T) {
			db := bootstrapDB(t)
			require.NoError(t, Install(db))
			plugin := "com.example.lifecycle"
			var p im.Permission
			if registrationOrder == "permission-first" {
				p = bootstrapPermission(t, db, plugin, "item")
			}
			f := bootstrapTenant(t, db, plugin, p, false)
			if registrationOrder == "plugin-first" {
				p = bootstrapPermission(t, db, plugin, "item")
			}
			track := trackingFor(t, f, "work.item:read")
			_, denied := f.current(t)
			require.Equal(t, Denied, denied.State)
			// A later real grant is native and is visible immediately after commit.
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				role := im.Role{TenantUUID: f.tenant, Scope: "tenant", Code: "reader"}
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
				if err := tx.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: p.ID, PermissionUUID: p.PermissionUUID}).Error; err != nil {
					return err
				}
				return tx.Create(&im.RoleBinding{TenantUUID: f.tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: f.member.ID, SubjectUUID: f.member.UUID.String(), DataScope: im.ScopeTenant}).Error
			}))
			_, granted := f.current(t)
			require.Equal(t, Allowed, granted.State)
			require.Equal(t, "native", granted.BaselineKind)
			require.NotNil(t, granted.GrantedAt)
			bootstrapPermission(t, db, plugin, "cost")
			require.Equal(t, AutomaticInitializer, trackingFor(t, f, "work.cost:read").InitializedBy)
			require.Equal(t, track, trackingFor(t, f, "work.item:read"))
			// Deactivation preserves the start; reenabling does not reset history.
			require.NoError(t, db.Model(&sm.PluginInstanceConfig{}).Where("tenant_uuid=?", f.tenant).Update("enabled", false).Error)
			require.NoError(t, db.Model(&sm.PluginInstanceConfig{}).Where("tenant_uuid=?", f.tenant).Update("enabled", true).Error)
			require.Equal(t, track, trackingFor(t, f, "work.item:read"))
		})
	}
}

func TestAutomaticTrackingRollbackAndFailureAreAtomic(t *testing.T) {
	db := bootstrapDB(t)
	f := bootstrapTenant(t, db, "com.example.rollback", im.Permission{}, false)
	require.NoError(t, Install(db))
	p := im.Permission{Module: "work", Resource: "item", Action: "read", Status: im.PermissionStatusActive, Source: "plugin:" + f.plugin, Meta: datatypes.JSON(`{"authorization_baseline":true}`)}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		return errors.New("cancelled")
	})
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&im.AuthorizationTracking{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_bootstrap", func(tx *gorm.DB) {
		if tx.Statement.Table == model.TableIAMAuthorizationSegment {
			tx.AddError(errors.New("cannot persist initial decision"))
		}
	}))
	p.ID = 0
	err = db.Create(&p).Error
	require.ErrorContains(t, err, "IAM_BASELINE_RECONCILE_FAILED")
	require.NoError(t, db.Model(&im.AuthorizationTracking{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&im.Permission{}).Count(&count).Error)
	require.Zero(t, count, "failed initialization must roll back the source registration")
}

func TestBootstrapSchemaAndUnavailableRowsAreNotSilentlyRepaired(t *testing.T) {
	db := bootstrapDB(t)
	f := bootstrapTenant(t, db, "com.example.blocked", bootstrapPermission(t, db, "com.example.blocked", "item"), false)
	blocked := im.AuthorizationTracking{TenantUUID: f.tenant, PluginID: f.plugin, PermissionCode: "work.item:read", Status: "blocked", InitializedBy: "audit"}
	require.NoError(t, db.Create(&blocked).Error)
	blocked = trackingFor(t, f, "work.item:read")
	require.NoError(t, Install(db))
	require.NoError(t, EnsureInitialized(db))
	require.Equal(t, blocked, trackingFor(t, f, "work.item:read"))
	require.NoError(t, db.Migrator().DropTable(&im.AuthorizationSegment{}))
	require.ErrorContains(t, EnsureInitialized(db), "IAM_BASELINE_SCHEMA_INCOMPLETE")
	require.NoError(t, db.Migrator().DropTable(&im.AuthorizationTracking{}))
	require.NoError(t, EnsureInitialized(db), "uninstalled database needs no writes")
}

func TestBootstrapInsideSourceTransactionKeepsItsCommitMaintenance(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&f.binding).Error; err != nil {
			return err
		}
		return EnsureInitialized(tx)
	}))
	_, decision := f.current(t)
	require.Equal(t, Denied, decision.State)
}
