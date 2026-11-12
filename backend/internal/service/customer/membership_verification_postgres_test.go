package customer

import (
	"os"
	"strings"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 仅连接专用临时库，绝不使用日常 Core 配置或迁移用户数据。
func TestMembershipVerificationPostgres(t *testing.T) {
	dsn := os.Getenv("POWERX_MEMBERSHIP_VERIFY_TEST_DSN")
	if dsn == "" {
		t.Skip("set POWERX_MEMBERSHIP_VERIFY_TEST_DSN to a dedicated px_customer_verify_ database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var name string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&name).Error)
	require.True(t, strings.HasPrefix(name, "px_customer_verify_"), "仅允许专用临时库")
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "public"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	require.NoError(t, db.AutoMigrate(&m.Account{}, &m.TenantMembership{}, &m.Contact{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.RoutingPolicy{}, &cm.AdapterEndpoint{}, &cm.FallbackPlan{}, &setting.PluginInstanceConfig{}, &im.Permission{}, &tenantmodel.Tenant{}))
	f := &verificationFixture{db: db, tenant: uuid.NewString(), customer: uuid.NewString(), membership: uuid.NewString(), contact: uuid.NewString(), now: time.Now().UTC().Truncate(time.Second)}
	f.seed(t)
	require.NoError(t, db.Create(&tenantmodel.Tenant{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(f.tenant)}, Key: "verification-fixture", Name: "Isolated verification", Status: 1}).Error)
	first, err := f.service.VerifyCustomer(f.ctx, f.customer)
	require.NoError(t, err)
	_, err = f.service.VerifyCustomer(f.ctx, uuid.NewString())
	require.ErrorIs(t, err, ErrVerificationNotFound)
	require.NoError(t, db.Model(&m.TenantMembership{}).Where("uuid=?", f.membership).Update("expires_at", f.now).Error)
	_, err = f.service.VerifyCustomer(f.ctx, f.customer)
	require.ErrorIs(t, err, ErrVerificationMembershipExpired)
	require.NoError(t, db.Model(&m.TenantMembership{}).Where("uuid=?", f.membership).Update("expires_at", nil).Error)
	// 不同租户的真实 Contact UUID 也不能被当作当前客户的主联系人。
	require.NoError(t, db.Model(&m.Contact{}).Where("uuid=?", f.contact).Update("tenant_uuid", uuid.NewString()).Error)
	_, err = f.service.VerifyCustomer(f.ctx, f.customer)
	require.ErrorIs(t, err, ErrVerificationPrimaryContactInvalid)
	require.NoError(t, db.Model(&m.Contact{}).Where("uuid=?", f.contact).Update("tenant_uuid", f.tenant).Error)
	second, err := f.service.VerifyCustomer(f.ctx, f.customer)
	require.NoError(t, err)
	require.NotEqual(t, first.Revision, second.Revision)
	// 原子切换主联系人并停用旧联系人；并发核验不得拼出跨快照的组合。
	other := uuid.NewString()
	require.NoError(t, db.Create(&m.Contact{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(other)}, TenantUUID: f.tenant, CustomerUUID: f.customer, DisplayName: "Other", Status: "inactive"}).Error)
	writerDone := make(chan error, 1)
	go func() {
		current, next := f.contact, other
		for n := 0; n < 40; n++ {
			err := db.Transaction(func(tx *gorm.DB) error {
				if e := tx.Model(&m.Contact{}).Where("uuid=?", next).Update("status", "active").Error; e != nil {
					return e
				}
				if e := tx.Model(&m.TenantMembership{}).Where("uuid=?", f.membership).Update("primary_contact_uuid", next).Error; e != nil {
					return e
				}
				return tx.Model(&m.Contact{}).Where("uuid=?", current).Update("status", "inactive").Error
			})
			if err != nil {
				writerDone <- err
				return
			}
			current, next = next, current
		}
		writerDone <- nil
	}()
	defer func() { require.NoError(t, <-writerDone) }()
	for n := 0; n < 60; n++ {
		result, e := f.service.VerifyCustomer(f.ctx, f.customer)
		require.NoError(t, e)
		require.Contains(t, []string{f.contact, other}, result.PrimaryContactUUID)
	}
	// 保留隔离库中的种子，供随后同一实例的 make capability-seed 验证。
}
