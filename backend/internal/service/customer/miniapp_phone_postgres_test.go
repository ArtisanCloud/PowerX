package customer

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func phonePostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("POWERX_CUSTOMER_PHONE_TEST_DSN")
	if dsn == "" {
		t.Skip("set POWERX_CUSTOMER_PHONE_TEST_DSN to an isolated PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(8)
	schema := "phone_verify_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	previous := coremodel.PowerXSchema
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		coremodel.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&modelcustomer.Account{}, &modelcustomer.TenantMembership{}, &modelcustomer.Contact{}, &modelaudit.AuditEvent{}))
	return db
}

func phonePostgresCustomer(t *testing.T, db *gorm.DB, tenant string) string {
	t.Helper()
	a := modelcustomer.Account{Type: modelcustomer.AccountTypePerson, Status: modelcustomer.StatusActive, DisplayName: "并发认证测试客户"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&modelcustomer.TenantMembership{TenantUUID: tenant, CustomerUUID: a.UUID.String(), Status: modelcustomer.StatusActive}).Error)
	return a.UUID.String()
}

func phonePostgresService(db *gorm.DB, verifier MiniAppPhoneVerifier) *CustomerAuthService {
	s := NewCustomerAuthService(db, nil).WithClientApps(NewStaticClientAppRegistry([]ClientApp{{Platform: WeChatMiniAppChannel, AppID: miniAppScoraAppID, Secret: "test-only", PluginIDs: []string{miniAppTestPlugin}}}))
	s.phoneVerifiers[WeChatMiniAppChannel] = verifier
	return s
}

func TestMiniAppPhonePostgresConcurrentConflict(t *testing.T) {
	db := phonePostgresDB(t)
	a := phonePostgresCustomer(t, db, miniAppTestTenant)
	b := phonePostgresCustomer(t, db, miniAppTestTenant)
	ready, release := make(chan struct{}, 2), make(chan struct{})
	verifier := miniAppPhoneVerifierFunc(func(ctx context.Context, _ ClientApp, _ string) (PhoneInfo, error) {
		ready <- struct{}{}
		select {
		case <-release:
			return PhoneInfo{PurePhone: "13800001234", CountryCode: "86"}, nil
		case <-ctx.Done():
			return PhoneInfo{}, ctx.Err()
		}
	})
	// 两个独立服务实例同时获得同号码，依靠数据库事务锁决出唯一归属。
	first, second := phonePostgresService(db, verifier), phonePostgresService(db.Session(&gorm.Session{NewDB: true}), verifier)
	ctx, cancel := context.WithTimeout(miniAppTestContext(miniAppTestPlugin, miniAppTestTenant), 10*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for i, customer := range []string{a, b} {
		s := []*CustomerAuthService{first, second}[i]
		go func() {
			_, err := s.VerifyMiniAppPhone(ctx, miniAppTestTenant, customer, WeChatMiniAppChannel, miniAppScoraAppID, "one-time-code")
			results <- err
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("并发请求未进入上游校验")
		}
	}
	close(release)
	success, conflict := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrCustomerPhoneConflict)
			conflict++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	var accounts []modelcustomer.Account
	require.NoError(t, db.Find(&accounts).Error)
	verified := 0
	for _, a := range accounts {
		if a.HasVerifiedPhone() {
			verified++
		}
	}
	require.Equal(t, 1, verified)
	var audits int64
	require.NoError(t, db.Model(&modelaudit.AuditEvent{}).Count(&audits).Error)
	require.EqualValues(t, 1, audits, "冲突请求无部分写入或成功审计")
	// 不同租户的另一个客户仍可使用该号码；锁的串行化不扩大冲突范围。
	c := phonePostgresCustomer(t, db, miniAppOtherTenant)
	independent := phonePostgresService(db, miniAppPhoneVerifierFunc(func(context.Context, ClientApp, string) (PhoneInfo, error) {
		return PhoneInfo{PurePhone: "13800001234", CountryCode: "86"}, nil
	}))
	_, err := independent.VerifyMiniAppPhone(miniAppTestContext(miniAppTestPlugin, miniAppOtherTenant), miniAppOtherTenant, c, WeChatMiniAppChannel, miniAppScoraAppID, "another-code")
	require.NoError(t, err)
	// 全局客户还属于其他租户时，改号也不能撞上那个租户的已有认证号码。
	thirdTenant := uuid.NewString()
	d := phonePostgresCustomer(t, db, thirdTenant)
	require.NoError(t, db.Create(&modelcustomer.TenantMembership{TenantUUID: miniAppOtherTenant, CustomerUUID: d, Status: modelcustomer.StatusActive}).Error)
	_, err = independent.VerifyMiniAppPhone(miniAppTestContext(miniAppTestPlugin, thirdTenant), thirdTenant, d, WeChatMiniAppChannel, miniAppScoraAppID, "shared-customer-code")
	require.ErrorIs(t, err, ErrCustomerPhoneConflict)
}

func TestMiniAppPhonePostgresRollbackReleasesNumber(t *testing.T) {
	db := phonePostgresDB(t)
	a := phonePostgresCustomer(t, db, miniAppTestTenant)
	b := phonePostgresCustomer(t, db, miniAppTestTenant)
	// 主联系人在事务中不存在，账户更新必须随之回滚，号码锁也必须释放。
	require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).Where("tenant_uuid = ? AND customer_uuid = ?", miniAppTestTenant, a).Update("primary_contact_uuid", uuid.NewString()).Error)
	s := phonePostgresService(db, miniAppPhoneVerifierFunc(func(context.Context, ClientApp, string) (PhoneInfo, error) {
		return PhoneInfo{PurePhone: "13800001234", CountryCode: "86"}, nil
	}))
	ctx, cancel := context.WithTimeout(miniAppTestContext(miniAppTestPlugin, miniAppTestTenant), 5*time.Second)
	defer cancel()
	_, err := s.VerifyMiniAppPhone(ctx, miniAppTestTenant, a, WeChatMiniAppChannel, miniAppScoraAppID, "first-code")
	require.ErrorIs(t, err, ErrCustomerUpstreamDependency)
	var account modelcustomer.Account
	require.NoError(t, db.Where("uuid = ?", a).First(&account).Error)
	require.Empty(t, account.PrimaryPhone)
	require.False(t, account.HasVerifiedPhone())
	_, err = s.VerifyMiniAppPhone(ctx, miniAppTestTenant, b, WeChatMiniAppChannel, miniAppScoraAppID, "second-code")
	require.NoError(t, err)
}
