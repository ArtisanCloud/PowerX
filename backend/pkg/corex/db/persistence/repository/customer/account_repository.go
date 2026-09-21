package customer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrExternalIdentityRequired            = errors.New("customer.external_identity_required")
	ErrExternalIdentityDisplayNameRequired = errors.New("customer.external_identity_display_name_required")
	ErrExternalIdentityServiceActorInvalid = errors.New("customer.external_identity_service_actor_invalid")
	ErrExternalIdentityBindingUntrusted    = errors.New("customer.external_identity_binding_untrusted")
	ErrExternalIdentityUnavailable         = errors.New("customer.external_identity_unavailable")
)

// ExternalIdentityInput is the plugin-attested channel-to-customer binding
// contract. ProviderKey and ProviderPluginID are derived by Core from the STS
// service actor; callers must never supply them in an invocation payload.
type ExternalIdentityInput struct {
	TenantUUID       string
	ProviderKey      string
	ProviderPluginID string
	ProviderSubject  string
	DisplayName      string
}

// ExternalIdentityProviderKey scopes a plugin-attested subject to exactly one
// plugin identity while fitting the persistent provider column's 32-char limit.
func ExternalIdentityProviderKey(pluginID string) (string, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return "", ErrExternalIdentityRequired
	}
	digest := sha256.Sum256([]byte(pluginID))
	return "plugin_" + hex.EncodeToString(digest[:])[:24], nil
}

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

// ExternalIdentityResolution is the deliberately minimal result available to a
// plugin identity resolver. It avoids loading or exposing customer PII.
type ExternalIdentityResolution struct {
	CustomerUUID   string `json:"customer_uuid"`
	MembershipUUID string `json:"membership_uuid"`
	DisplayName    string `json:"display_name"`
}

type OverviewRow struct {
	Total     int64 `json:"total"`
	Active    int64 `json:"active"`
	Pending   int64 `json:"pending"`
	Suspended int64 `json:"suspended"`
	Disabled  int64 `json:"disabled"`
}

// CustomerIdentityRow deliberately omits authentication secrets. It is the
// admin projection of an authentication identity, not a ContactIdentity.
type CustomerIdentityRow struct {
	UUID            string     `json:"uuid"`
	CustomerUUID    string     `json:"customer_uuid"`
	Provider        string     `json:"provider"`
	ProviderSubject string     `json:"provider_subject,omitempty"`
	Email           string     `json:"email,omitempty"`
	Phone           string     `json:"phone,omitempty"`
	Status          string     `json:"status"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
}

// CurrentMembershipRow contains only the authorization data required by a
// customer self/delegated membership check; it intentionally excludes PII.
type CurrentMembershipRow struct {
	TenantUUID     string
	CustomerUUID   string
	MembershipUUID string
	Status         string
	AccountStatus  string
	Roles          datatypes.JSON
	Scopes         datatypes.JSON
	ExpiresAt      *time.Time
}

func (r *AccountRepository) CurrentMembership(ctx context.Context, tenantUUID, customerUUID string) (CurrentMembershipRow, error) {
	if r == nil || r.db == nil {
		return CurrentMembershipRow{}, gorm.ErrInvalidDB
	}
	var row CurrentMembershipRow
	err := r.baseTenantQuery(ctx, tenantUUID).
		Where("a.uuid = ?", customerUUID).
		Select(`m.tenant_uuid AS tenant_uuid, m.customer_uuid AS customer_uuid, m.uuid AS membership_uuid,
			m.status AS status, a.status AS account_status, m.roles AS roles, m.scopes AS scopes, m.expires_at AS expires_at`).
		Limit(1).Scan(&row).Error
	if err != nil {
		return CurrentMembershipRow{}, err
	}
	if row.MembershipUUID == "" {
		return CurrentMembershipRow{}, gorm.ErrRecordNotFound
	}
	return row, nil
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

// Get returns an account only when it is a member of the requested tenant.
// A global Customer UUID alone is never sufficient to read its profile.
func (r *AccountRepository) Get(ctx context.Context, tenantUUID, customerUUID string) (AccountRow, error) {
	if r == nil || r.db == nil {
		return AccountRow{}, gorm.ErrInvalidDB
	}
	var row AccountRow
	err := r.baseTenantQuery(ctx, tenantUUID).
		Where("a.uuid = ?", strings.TrimSpace(customerUUID)).
		Select(`a.uuid::text AS uuid,
			a.status AS status, a.primary_email AS primary_email, a.primary_phone AS primary_phone,
			a.display_name AS display_name, a.nickname AS nickname, a.given_name AS given_name,
			a.family_name AS family_name, a.avatar_url AS avatar_url, a.locale AS locale, a.timezone AS timezone,
			m.status AS member_status, m.source AS member_source, m.uuid::text AS membership_uuid,
			a.created_at::text AS created_at, a.updated_at::text AS updated_at`).
		Limit(1).Scan(&row).Error
	if err != nil {
		return AccountRow{}, err
	}
	if row.UUID == "" {
		return AccountRow{}, gorm.ErrRecordNotFound
	}
	return row, nil
}

func (r *AccountRepository) ListAuthIdentities(ctx context.Context, tenantUUID, customerUUID string) ([]CustomerIdentityRow, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var rows []CustomerIdentityRow
	err := r.db.WithContext(ctx).Table((modelcustomer.AuthIdentity{}).TableName()+" AS i").
		Joins("JOIN "+(modelcustomer.TenantMembership{}).TableName()+" AS m ON m.customer_uuid = i.customer_uuid").
		Where("m.tenant_uuid = ? AND m.customer_uuid = ? AND m.deleted_at IS NULL", strings.TrimSpace(tenantUUID), strings.TrimSpace(customerUUID)).
		Select(`i.uuid::text AS uuid, i.customer_uuid::text AS customer_uuid, i.provider AS provider,
			i.provider_subject AS provider_subject, i.email AS email, i.phone AS phone, i.status AS status,
			i.verified_at AS verified_at, i.created_at::text AS created_at, i.updated_at::text AS updated_at`).
		Order("i.created_at DESC").Scan(&rows).Error
	return rows, err
}

func (r *AccountRepository) ListMemberships(ctx context.Context, tenantUUID, customerUUID string) ([]modelcustomer.TenantMembership, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var rows []modelcustomer.TenantMembership
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND customer_uuid = ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(customerUUID)).Order("created_at DESC").Find(&rows).Error
	return rows, err
}

func (r *AccountRepository) ListLoginEvents(ctx context.Context, tenantUUID, customerUUID string, page, pageSize int) ([]modelcustomer.LoginEvent, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	query := r.db.WithContext(ctx).Model(&modelcustomer.LoginEvent{}).Where("tenant_uuid = ? AND customer_uuid = ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(customerUUID))
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var rows []modelcustomer.LoginEvent
	err := query.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, total, err
}

func (r *AccountRepository) ListMiniAppEntries(ctx context.Context, tenantUUID string, page, pageSize int) ([]modelcustomer.MiniAppEntry, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	query := r.db.WithContext(ctx).Model(&modelcustomer.MiniAppEntry{}).Where("tenant_uuid = ?", strings.TrimSpace(tenantUUID))
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var rows []modelcustomer.MiniAppEntry
	err := query.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, total, err
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

// ResolveOrCreateExternalIdentity returns the single Core customer associated
// with an external provider subject and ensures its tenant membership exists.
func (r *AccountRepository) ResolveOrCreateExternalIdentity(ctx context.Context, in ExternalIdentityInput) (ExternalIdentityResolution, error) {
	if r == nil || r.db == nil {
		return ExternalIdentityResolution{}, gorm.ErrInvalidDB
	}
	in.TenantUUID = strings.TrimSpace(in.TenantUUID)
	in.ProviderKey = strings.TrimSpace(in.ProviderKey)
	in.ProviderPluginID = strings.TrimSpace(in.ProviderPluginID)
	in.ProviderSubject = strings.TrimSpace(in.ProviderSubject)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.TenantUUID == "" || in.ProviderKey == "" || in.ProviderPluginID == "" || in.ProviderSubject == "" {
		return ExternalIdentityResolution{}, ErrExternalIdentityRequired
	}
	if in.DisplayName == "" {
		return ExternalIdentityResolution{}, ErrExternalIdentityDisplayNameRequired
	}
	if len(in.ProviderKey) > 32 || len(in.ProviderSubject) > 255 || len(in.DisplayName) > 128 {
		return ExternalIdentityResolution{}, ErrExternalIdentityRequired
	}
	var customerUUID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity modelcustomer.AuthIdentity
		err := tx.Where("provider = ? AND provider_subject = ?", in.ProviderKey, in.ProviderSubject).First(&identity).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			customerUUID = uuid.NewString()
			candidate := modelcustomer.AuthIdentity{
				CustomerUUID: customerUUID,
				Provider:     in.ProviderKey, ProviderSubject: in.ProviderSubject,
				Status:   modelcustomer.StatusActive,
				Metadata: externalIdentityMetadata(in.ProviderPluginID, in.ProviderKey),
			}
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "provider"}, {Name: "provider_subject"}},
				DoNothing: true,
			}).Create(&candidate)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				if err := tx.Where("provider = ? AND provider_subject = ?", in.ProviderKey, in.ProviderSubject).First(&identity).Error; err != nil {
					return err
				}
			} else {
				accountUUID, parseErr := uuid.Parse(customerUUID)
				if parseErr != nil {
					return parseErr
				}
				account := &modelcustomer.Account{
					PowerUUIDModel: coremodel.PowerUUIDModel{UUID: accountUUID},
					Status:         modelcustomer.StatusActive, DisplayName: in.DisplayName,
					Metadata: externalIdentityMetadata(in.ProviderPluginID, in.ProviderKey),
				}
				if err := tx.Create(account).Error; err != nil {
					return err
				}
				identity = candidate
			}
		}
		customerUUID = identity.CustomerUUID
		if identity.Status != modelcustomer.StatusActive || customerUUID == "" || !isPluginAttestedIdentity(identity, in.ProviderPluginID, in.ProviderKey) {
			return ErrExternalIdentityBindingUntrusted
		}
		var account modelcustomer.Account
		if err := tx.Where("uuid = ?", customerUUID).First(&account).Error; err != nil {
			return err
		}
		if account.Status != modelcustomer.StatusActive {
			return ErrExternalIdentityUnavailable
		}
		var membership modelcustomer.TenantMembership
		err = tx.Where("tenant_uuid = ? AND customer_uuid = ?", in.TenantUUID, customerUUID).First(&membership).Error
		if err == nil {
			if membership.Status != modelcustomer.StatusActive {
				return ErrExternalIdentityUnavailable
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		created := modelcustomer.TenantMembership{
			TenantUUID: in.TenantUUID, CustomerUUID: customerUUID, Status: modelcustomer.StatusActive,
			Source: "plugin_identity", Roles: datatypes.JSON([]byte("[\"customer\"]")), Scopes: datatypes.JSON([]byte("[]")),
			Metadata: externalIdentityMetadata(in.ProviderPluginID, in.ProviderKey),
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_uuid"}, {Name: "customer_uuid"}},
			DoNothing: true,
		}).Create(&created)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Where("tenant_uuid = ? AND customer_uuid = ?", in.TenantUUID, customerUUID).First(&membership).Error; err != nil {
				return err
			}
			if membership.Status != modelcustomer.StatusActive {
				return ErrExternalIdentityUnavailable
			}
		}
		return nil
	})
	if err != nil {
		return ExternalIdentityResolution{}, err
	}
	return r.resolvedExternalIdentityRow(ctx, in.TenantUUID, customerUUID)
}

func (r *AccountRepository) resolvedExternalIdentityRow(ctx context.Context, tenantUUID, customerUUID string) (ExternalIdentityResolution, error) {
	var row ExternalIdentityResolution
	err := r.baseTenantQuery(ctx, tenantUUID).
		Where("a.uuid = ?", customerUUID).
		Select(`a.uuid AS customer_uuid,
			a.display_name AS display_name,
			m.uuid AS membership_uuid`).
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return ExternalIdentityResolution{}, err
	}
	if row.CustomerUUID == "" {
		return ExternalIdentityResolution{}, gorm.ErrRecordNotFound
	}
	return row, nil
}

func externalIdentityMetadata(pluginID, providerKey string) datatypes.JSON {
	raw, err := json.Marshal(map[string]string{
		"binding_kind":       "plugin_attested",
		"provider_plugin_id": pluginID,
		"provider_key":       providerKey,
	})
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(raw)
}

func isPluginAttestedIdentity(identity modelcustomer.AuthIdentity, pluginID, providerKey string) bool {
	if identity.VerifiedAt != nil {
		return false
	}
	var metadata map[string]string
	if err := json.Unmarshal(identity.Metadata, &metadata); err != nil {
		return false
	}
	return metadata["binding_kind"] == "plugin_attested" &&
		metadata["provider_plugin_id"] == pluginID &&
		metadata["provider_key"] == providerKey
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
