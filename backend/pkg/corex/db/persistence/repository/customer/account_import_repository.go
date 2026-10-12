package customer

import (
	"context"
	"hash/fnv"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AccountImportRepository struct {
	*base.BaseRepository[modelcustomer.AccountImport]
}

func NewAccountImportRepository(db *gorm.DB) *AccountImportRepository {
	return &AccountImportRepository{BaseRepository: base.NewBaseRepository[modelcustomer.AccountImport](db)}
}

func (r *AccountImportRepository) Get(ctx context.Context, tenant string, actor uint64, id string, lock bool) (modelcustomer.AccountImport, error) {
	var row modelcustomer.AccountImport
	q := r.DB.WithContext(ctx).Where("tenant_uuid = ? AND actor_user_id = ? AND uuid = ?", tenant, actor, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&row).Error
	return row, err
}

// LockTenant 串行化同租户的导入提交，避免两个不同预览同时通过重复检查。
func (r *AccountImportRepository) LockTenant(ctx context.Context, tenant string) error {
	if r.DB.Dialector.Name() != "postgres" {
		return nil
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte("customer-account-import:" + tenant))
	return r.DB.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", int64(h.Sum64())).Error
}

func (r *AccountImportRepository) IdentityExists(ctx context.Context, tenant, email, phone string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	phone = NormalizeImportPhone(phone)
	if email == "" && phone == "" {
		return false, nil
	}
	q := r.DB.WithContext(ctx).Table(coremodel.PowerXSchema+"."+coremodel.TableCustomerAccounts+" AS a").
		Joins("JOIN "+coremodel.PowerXSchema+"."+coremodel.TableCustomerTenantMemberships+" AS m ON m.customer_uuid = a.uuid").
		Where("m.tenant_uuid = ? AND m.deleted_at IS NULL AND a.deleted_at IS NULL", tenant)
	conditions, args := []string{}, []any{}
	if email != "" {
		conditions = append(conditions, "LOWER(a.primary_email) = ?")
		args = append(args, email)
	}
	if phone != "" {
		conditions = append(conditions, "REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(a.primary_phone, ' ', ''), '+', ''), '-', ''), '(', ''), ')', '') = ?")
		args = append(args, phone)
	}
	var found int
	err := q.Where("("+strings.Join(conditions, " OR ")+")", args...).Select("1").Limit(1).Scan(&found).Error
	return found == 1, err
}

func NormalizeImportPhone(phone string) string {
	return strings.NewReplacer(" ", "", "+", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(phone))
}

func (r *AccountImportRepository) SaveResult(ctx context.Context, row *modelcustomer.AccountImport) error {
	return r.DB.WithContext(ctx).Model(row).Where("tenant_uuid = ? AND actor_user_id = ?", row.TenantUUID, row.ActorUserID).
		Updates(map[string]any{"rows": row.Rows, "status": row.Status, "trace_id": row.TraceID}).Error
}

func (r *AccountImportRepository) RecordAudit(ctx context.Context, event *modelaudit.AuditEvent) error {
	return r.DB.WithContext(ctx).Create(event).Error
}
