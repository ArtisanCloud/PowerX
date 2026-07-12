package customer

import (
	"context"
	"strings"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"gorm.io/gorm"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

type AccountListOptions struct {
	TenantUUID string
	Query      string
	Status     string
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

type AccountRow struct {
	UUID           string `json:"uuid"`
	Status         string `json:"status"`
	PrimaryEmail   string `json:"primary_email,omitempty"`
	PrimaryPhone   string `json:"primary_phone,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	Nickname       string `json:"nickname,omitempty"`
	GivenName      string `json:"given_name,omitempty"`
	FamilyName     string `json:"family_name,omitempty"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	Locale         string `json:"locale,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	MemberStatus   string `json:"member_status"`
	MemberSource   string `json:"member_source"`
	MembershipUUID string `json:"membership_uuid"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type OverviewRow struct {
	Total     int64 `json:"total"`
	Active    int64 `json:"active"`
	Pending   int64 `json:"pending"`
	Suspended int64 `json:"suspended"`
	Disabled  int64 `json:"disabled"`
}

func (r *AccountRepository) List(ctx context.Context, opt AccountListOptions) ([]AccountRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	query := r.baseTenantQuery(ctx, opt.TenantUUID)
	query = applyAccountFilters(query, opt)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := opt.Page
	if page <= 0 {
		page = 1
	}
	pageSize := opt.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	sortBy := sanitizeAccountSortBy(opt.SortBy)
	sortOrder := strings.ToLower(strings.TrimSpace(opt.SortOrder))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	var rows []AccountRow
	err := query.
		Select(`a.uuid::text AS uuid,
			a.status AS status,
			a.primary_email AS primary_email,
			a.primary_phone AS primary_phone,
			a.display_name AS display_name,
			a.nickname AS nickname,
			a.given_name AS given_name,
			a.family_name AS family_name,
			a.avatar_url AS avatar_url,
			a.locale AS locale,
			a.timezone AS timezone,
			m.status AS member_status,
			m.source AS member_source,
			m.uuid::text AS membership_uuid,
			a.created_at::text AS created_at,
			a.updated_at::text AS updated_at`).
		Order("a." + sortBy + " " + sortOrder).
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *AccountRepository) Overview(ctx context.Context, tenantUUID string) (OverviewRow, error) {
	var out OverviewRow
	if r == nil || r.db == nil {
		return out, gorm.ErrInvalidDB
	}
	rows, err := r.baseTenantQuery(ctx, tenantUUID).
		Select(`a.status AS status, count(*) AS count`).
		Group("a.status").
		Rows()
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return out, err
		}
		out.Total += count
		switch status {
		case modelcustomer.StatusActive:
			out.Active = count
		case modelcustomer.StatusPending:
			out.Pending = count
		case modelcustomer.StatusSuspended:
			out.Suspended = count
		case modelcustomer.StatusDisabled:
			out.Disabled = count
		}
	}
	return out, rows.Err()
}

func (r *AccountRepository) CreateWithMembership(ctx context.Context, tenantUUID string, account *modelcustomer.Account, membership *modelcustomer.TenantMembership) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(account).Error; err != nil {
			return err
		}
		membership.TenantUUID = tenantUUID
		membership.CustomerUUID = account.UUID.String()
		if err := tx.Create(membership).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *AccountRepository) UpdateTenantStatus(ctx context.Context, tenantUUID string, customerUUID string, status string) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		accountResult := tx.Model(&modelcustomer.Account{}).
			Where("uuid = ?", customerUUID).
			Update("status", status)
		if accountResult.Error != nil {
			return accountResult.Error
		}
		if accountResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		memberResult := tx.Model(&modelcustomer.TenantMembership{}).
			Where("tenant_uuid = ? AND customer_uuid = ?", tenantUUID, customerUUID).
			Update("status", status)
		if memberResult.Error != nil {
			return memberResult.Error
		}
		if memberResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *AccountRepository) baseTenantQuery(ctx context.Context, tenantUUID string) *gorm.DB {
	accountTable := (modelcustomer.Account{}).TableName()
	memberTable := (modelcustomer.TenantMembership{}).TableName()
	return r.db.WithContext(ctx).
		Table(accountTable+" AS a").
		Joins("JOIN "+memberTable+" AS m ON m.customer_uuid = a.uuid").
		Where("m.tenant_uuid = ?", tenantUUID).
		Where("a.deleted_at IS NULL").
		Where("m.deleted_at IS NULL")
}

func applyAccountFilters(query *gorm.DB, opt AccountListOptions) *gorm.DB {
	if status := strings.TrimSpace(opt.Status); status != "" {
		query = query.Where("a.status = ?", status)
	}
	if q := strings.TrimSpace(opt.Query); q != "" {
		like := "%" + q + "%"
		query = query.Where(
			"a.uuid::text = ? OR a.display_name ILIKE ? OR a.nickname ILIKE ? OR a.primary_email ILIKE ? OR a.primary_phone ILIKE ?",
			q, like, like, like, like,
		)
	}
	return query
}

func sanitizeAccountSortBy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "display_name":
		return "display_name"
	case "status":
		return "status"
	case "updated_at":
		return "updated_at"
	default:
		return "created_at"
	}
}
