package customer

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestClientApplicationPostgresConcurrentSave(t *testing.T) {
	db := phonePostgresDB(t)
	require.NoError(t, database.MigrateCustomerClientAppModels(db))
	require.NoError(t, database.MigrateCustomerClientAppModels(db), "scoped migration is idempotent")
	require.NoError(t, db.AutoMigrate(&setting.PluginInstanceConfig{}, &tenantmodel.TenantKeyPair{}))
	tenant := uuid.NewString()
	ctx := importAdminContext(tenant)
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: "com.example.shop", Key: "auth.credentials", Enabled: true, Status: "enabled"}).Error)
	wrapper, err := tenantkeys.NewDirectWrapper("pg-client-app-test", "", []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	service := NewClientApplicationService(db, tenantkeys.NewTenantKeyServiceWithWrapper(db, wrapper))
	initial, err := service.Save(ctx, "", clientAppInput("initial-secret"))
	require.NoError(t, err)
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			ready.Done()
			<-start
			in := clientAppUpdate(initial)
			in.Name = "Concurrent update"
			_, e := service.Save(ctx, initial.UUID, in)
			results <- e
		}()
	}
	ready.Wait()
	close(start)
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		e := <-results
		if e == nil {
			successes++
		} else {
			require.Equal(t, "CLIENT_APP_VERSION_CONFLICT", dto.CodeOf(e))
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	var count int64
	require.NoError(t, db.Model(&model.ClientAppBinding{}).Where("tenant_uuid = ?", tenant).Count(&count).Error)
	require.EqualValues(t, 1, count)
	_, err = service.Save(ctx, "", clientAppInput("other-secret"))
	require.Equal(t, "CLIENT_APP_DUPLICATE", dto.CodeOf(err))
	fresh := NewClientApplicationService(db, tenantkeys.NewTenantKeyServiceWithWrapper(db, wrapper))
	app, ok, err := fresh.Lookup(ctx, tenant, "com.example.shop", WeChatMiniAppChannel, "wx-test-application")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "initial-secret", app.Secret)
	require.EqualValues(t, 2, app.CredentialVersion)
}

func TestClientApplicationPostgresRotationReadsOneRevision(t *testing.T) {
	db := phonePostgresDB(t)
	require.NoError(t, database.MigrateCustomerClientAppModels(db))
	require.NoError(t, db.AutoMigrate(&setting.PluginInstanceConfig{}, &tenantmodel.TenantKeyPair{}))
	tenant := uuid.NewString()
	ctx := importAdminContext(tenant)
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: "com.example.shop", Key: "auth.credentials", Enabled: true, Status: "enabled"}).Error)
	wrapper, err := tenantkeys.NewDirectWrapper("rotation-test", "", []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	service := NewClientApplicationService(db, tenantkeys.NewTenantKeyServiceWithWrapper(db, wrapper))
	current, err := service.Save(ctx, "", clientAppInput("revision-1"))
	require.NoError(t, err)
	stop, readerDone := make(chan struct{}), make(chan struct{})
	failures := make(chan error, 1)
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			app, ok, e := service.Lookup(context.Background(), tenant, "com.example.shop", WeChatMiniAppChannel, "wx-test-application")
			if e != nil || !ok || app.Secret != fmt.Sprintf("revision-%d", app.CredentialVersion) {
				failures <- fmt.Errorf("credential revision mismatch: version=%d found=%t error=%v", app.CredentialVersion, ok, e)
				return
			}
		}
	}()
	defer func() {
		close(stop)
		<-readerDone
		select {
		case e := <-failures:
			t.Error(e)
		default:
		}
	}()
	for version := 2; version <= 12; version++ {
		in := clientAppUpdate(current)
		secret := fmt.Sprintf("revision-%d", version)
		in.Connections[0].AppSecret = &secret
		current, err = service.Save(ctx, current.UUID, in)
		require.NoError(t, err)
	}
}

func TestClientApplicationPostgresFirstCredentialsAreConcurrentAndRotatable(t *testing.T) {
	db := phonePostgresDB(t)
	require.NoError(t, database.MigrateCustomerClientAppModels(db))
	require.NoError(t, db.AutoMigrate(&setting.PluginInstanceConfig{}, &tenantmodel.TenantKeyPair{}))
	tenant := uuid.NewString()
	ctx := importAdminContext(tenant)
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: "com.example.shop", Key: "auth.credentials", Enabled: true, Status: "enabled"}).Error)
	wrapper, err := tenantkeys.NewDirectWrapper("first-credentials-test", "", []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	keys := tenantkeys.NewTenantKeyServiceWithWrapper(db, wrapper)
	service := NewClientApplicationService(db, keys)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []string{"wx-first-one", "wx-first-two"} {
		go func(id string) {
			<-start
			in := clientAppInput("credential-" + id)
			in.Connections[0].AppID = id
			_, e := service.Save(ctx, "", in)
			results <- e
		}(id)
	}
	close(start)
	for i := 0; i < 2; i++ {
		require.NoError(t, <-results)
	}
	check := func() {
		for _, id := range []string{"wx-first-one", "wx-first-two"} {
			app, ok, e := service.Lookup(ctx, tenant, "com.example.shop", WeChatMiniAppChannel, id)
			require.NoError(t, e)
			require.True(t, ok)
			require.Equal(t, "credential-"+id, app.Secret)
		}
	}
	check()
	_, err = keys.RotateKeyPair(ctx, "default", tenant)
	require.NoError(t, err)
	check()
}
