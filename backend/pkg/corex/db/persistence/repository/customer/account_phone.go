package customer

import (
	"context"
	"encoding/json"

	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LockPhoneVerification 在调用方事务内串行化同号码认证，跨进程生效，事务结束自动释放。
// Account 是跨租户主体，因此锁按号码建立；SQLite 测试由数据库自身串行化写事务。
func (r *AccountRepository) LockPhoneVerification(ctx context.Context, phone string) error {
	if r.db.Dialector.Name() != "postgres" {
		return nil
	}
	return r.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "customer:verified-phone:"+phone).Error
}

// LockPhoneAccount 与资料编辑保持相同的成员关系、账户加锁顺序。
func (r *AccountRepository) LockPhoneAccount(ctx context.Context, tenant, customer string) (modelcustomer.Account, modelcustomer.TenantMembership, error) {
	var membership modelcustomer.TenantMembership
	var account modelcustomer.Account
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND customer_uuid = ?", tenant, customer).First(&membership).Error; err != nil {
		return account, membership, err
	}
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ?", customer).First(&account).Error
	return account, membership, err
}

// PhoneAccount 读取全局客户的当前号码及认证证据。
func (r *AccountRepository) PhoneAccount(ctx context.Context, customer string) (modelcustomer.Account, error) {
	var account modelcustomer.Account
	err := r.db.WithContext(ctx).Select("primary_phone", "metadata").Where("uuid = ?", customer).First(&account).Error
	return account, err
}

// OtherPhoneAccounts 检查此次全局号码更新会影响的所有已有租户成员关系。
func (r *AccountRepository) OtherPhoneAccounts(ctx context.Context, tenant, customer, phone string) ([]modelcustomer.Account, error) {
	var others []modelcustomer.Account
	tenants := r.db.Model(&modelcustomer.TenantMembership{}).Select("tenant_uuid").Where("customer_uuid = ?", customer)
	members := r.db.Model(&modelcustomer.TenantMembership{}).Select("customer_uuid").Where("tenant_uuid = ? OR tenant_uuid IN (?)", tenant, tenants)
	err := r.db.WithContext(ctx).Where("primary_phone = ? AND uuid <> ? AND uuid IN (?)", phone, customer, members).Find(&others).Error
	return others, err
}

// SaveVerifiedPhone 只能在持有成员关系与账户行锁的事务中调用。
func (r *AccountRepository) SaveVerifiedPhone(ctx context.Context, account modelcustomer.Account, membership modelcustomer.TenantMembership, phone string, metadata datatypes.JSON, audit *modelaudit.AuditEvent) error {
	if err := r.db.WithContext(ctx).Model(&account).Updates(map[string]any{"primary_phone": phone, "metadata": metadata}).Error; err != nil {
		return err
	}
	if membership.PrimaryContactUUID != "" {
		result := r.db.WithContext(ctx).Model(&modelcustomer.Contact{}).
			Where("uuid = ? AND tenant_uuid = ? AND customer_uuid = ? AND status = ?", membership.PrimaryContactUUID, membership.TenantUUID, membership.CustomerUUID, modelcustomer.ContactStatusActive).
			Update("phone", phone)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
	}
	return r.db.WithContext(ctx).Create(audit).Error
}

// invalidateChangedPhone 清除旧号码认证，避免改回旧号时重新激活已失效的证据。
func invalidateChangedPhone(account modelcustomer.Account, updates map[string]any) error {
	next, changed := updates["primary_phone"]
	if !changed || next == account.PrimaryPhone {
		return nil
	}
	metadata := map[string]any{}
	if len(account.Metadata) > 0 {
		if err := json.Unmarshal(account.Metadata, &metadata); err != nil {
			return err
		}
	}
	delete(metadata, modelcustomer.PhoneVerifiedAtKey)
	delete(metadata, modelcustomer.PhoneVerifiedViaKey)
	delete(metadata, modelcustomer.PhoneVerifiedHashKey)
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err == nil {
		updates["metadata"] = datatypes.JSON(raw)
	}
	return err
}
