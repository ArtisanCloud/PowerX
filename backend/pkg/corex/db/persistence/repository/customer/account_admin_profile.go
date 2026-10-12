package customer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAccountProfileChanged = errors.New("customer.account_profile_changed")

// AccountProfileVersion 只绑定可编辑的主资料；状态和联系人另有独立更新合同。
func AccountProfileVersion(id, displayName, nickname, email, phone string) string {
	data, _ := json.Marshal([]string{id, displayName, nickname, email, phone})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// UpdateAdminProfile 在调用方事务中锁定成员关系和客户，校验旧资料版本后只更新四个字段。
func (r *AccountRepository) UpdateAdminProfile(ctx context.Context, tenant, id, expected string, fields map[string]any) error {
	var membership modelcustomer.TenantMembership
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_uuid = ? AND customer_uuid = ?", tenant, id).First(&membership).Error; err != nil {
		return err
	}
	var account modelcustomer.Account
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ?", id).First(&account).Error; err != nil {
		return err
	}
	if AccountProfileVersion(id, account.DisplayName, account.Nickname, account.PrimaryEmail, account.PrimaryPhone) != expected {
		return ErrAccountProfileChanged
	}
	updates := map[string]any{}
	for _, key := range []string{"display_name", "nickname", "primary_email", "primary_phone"} {
		if value, ok := fields[key]; ok {
			updates[key] = value
		}
	}
	if len(updates) == 0 {
		return gorm.ErrInvalidData
	}
	return r.db.WithContext(ctx).Model(&account).Updates(updates).Error
}
