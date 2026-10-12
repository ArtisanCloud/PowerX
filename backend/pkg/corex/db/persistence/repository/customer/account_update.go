package customer

import (
	"context"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpdateProfile 在调用方事务内锁定租户成员关系，验证归属后更新资料白名单。
func (r *AccountRepository) UpdateProfile(ctx context.Context, tenantUUID, customerUUID string, fields map[string]any) error {
	var membership modelcustomer.TenantMembership
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND customer_uuid = ?", tenantUUID, customerUUID).First(&membership).Error; err != nil {
		return err
	}
	updates := map[string]any{}
	for _, key := range []string{"display_name", "nickname", "given_name", "family_name", "primary_email", "primary_phone", "avatar_url", "locale", "timezone", "status"} {
		if value, ok := fields[key]; ok {
			updates[key] = value
		}
	}
	result := r.db.WithContext(ctx).Model(&modelcustomer.Account{}).Where("uuid = ?", customerUUID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	if status, ok := updates["status"]; ok {
		if err := r.db.WithContext(ctx).Model(&membership).Update("status", status).Error; err != nil {
			return err
		}
	}
	var account modelcustomer.Account
	if err := r.db.WithContext(ctx).Where("uuid = ?", customerUUID).First(&account).Error; err != nil {
		return err
	}
	return ensureExternalIdentityPrimaryContact(r.db.WithContext(ctx), ExternalIdentityInput{TenantUUID: tenantUUID}, &account, &membership)
}
