package customer

import (
	"context"
	"time"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"gorm.io/gorm"
)

// MembershipVerificationSnapshot 只包含核验和快照摘要所需数据，不读取联系人资料。
type MembershipVerificationSnapshot struct {
	TenantUUID, CustomerUUID, MembershipUUID, PrimaryContactUUID string
	AccountStatus, MembershipStatus, ContactUUID, ContactStatus  string
	ExpiresAt                                                    *time.Time
	AccountUpdatedAt, MembershipUpdatedAt                        time.Time
	ContactUpdatedAt                                             *time.Time
}

// MembershipVerification 在一条 SQL 的一致性快照中读取三个对象。
// Contact 使用带租户、客户和软删条件的 LEFT JOIN，失效引用不会被误判为客户不存在。
func (r *AccountRepository) MembershipVerification(ctx context.Context, tenantUUID, customerUUID string) (MembershipVerificationSnapshot, error) {
	if r == nil || r.db == nil {
		return MembershipVerificationSnapshot{}, gorm.ErrInvalidDB
	}
	var row MembershipVerificationSnapshot
	err := r.baseTenantQuery(ctx, tenantUUID).
		Joins("LEFT JOIN "+(modelcustomer.Contact{}).TableName()+" AS c ON c.uuid = m.primary_contact_uuid AND c.tenant_uuid = m.tenant_uuid AND c.customer_uuid = m.customer_uuid AND c.deleted_at IS NULL").
		Where("a.uuid = ?", customerUUID).
		Select(`m.tenant_uuid, m.customer_uuid, m.uuid AS membership_uuid, m.primary_contact_uuid,
			a.status AS account_status, m.status AS membership_status, m.expires_at,
			c.uuid AS contact_uuid, c.status AS contact_status,
			a.updated_at AS account_updated_at, m.updated_at AS membership_updated_at, c.updated_at AS contact_updated_at`).
		Limit(1).Scan(&row).Error
	if err != nil {
		return MembershipVerificationSnapshot{}, err
	}
	if row.MembershipUUID == "" {
		return MembershipVerificationSnapshot{}, gorm.ErrRecordNotFound
	}
	return row, nil
}
