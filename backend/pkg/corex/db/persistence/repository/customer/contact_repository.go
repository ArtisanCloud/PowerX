package customer

import (
	"context"
	"strings"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"gorm.io/gorm"
)

// ContactRepository contains only tenant- and Customer-scoped persistence
// operations. Contact ownership, status, and channel validation belong to the
// service layer.
type ContactRepository struct {
	db *gorm.DB
}

func NewContactRepository(db *gorm.DB) *ContactRepository {
	return &ContactRepository{db: db}
}

type ContactListOptions struct {
	TenantUUID   string
	CustomerUUID string
	Query        string
	Status       string
	Page         int
	PageSize     int
}

func (r *ContactRepository) Create(ctx context.Context, contact *modelcustomer.Contact) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	return r.db.WithContext(ctx).Create(contact).Error
}

func (r *ContactRepository) Get(ctx context.Context, tenantUUID, customerUUID, contactUUID string) (*modelcustomer.Contact, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var contact modelcustomer.Contact
	if err := r.scopedContacts(ctx, tenantUUID, customerUUID).
		Where("uuid = ?", strings.TrimSpace(contactUUID)).
		First(&contact).Error; err != nil {
		return nil, err
	}
	return &contact, nil
}

// FindByTenant is intentionally narrower than a public get operation. The
// service uses it only after a scoped miss to distinguish a same-tenant
// Customer mismatch from an absent Contact without ever crossing tenants.
func (r *ContactRepository) FindByTenant(ctx context.Context, tenantUUID, contactUUID string) (*modelcustomer.Contact, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var contact modelcustomer.Contact
	if err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND uuid = ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(contactUUID)).
		First(&contact).Error; err != nil {
		return nil, err
	}
	return &contact, nil
}

func (r *ContactRepository) List(ctx context.Context, options ContactListOptions) ([]modelcustomer.Contact, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	query := r.scopedContacts(ctx, options.TenantUUID, options.CustomerUUID)
	if status := strings.TrimSpace(options.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if value := strings.TrimSpace(options.Query); value != "" {
		query = query.Where("display_name LIKE ? OR given_name LIKE ? OR family_name LIKE ?", "%"+value+"%", "%"+value+"%", "%"+value+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := options.Page
	if page <= 0 {
		page = 1
	}
	pageSize := options.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var contacts []modelcustomer.Contact
	if err := query.Select("*").Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&contacts).Error; err != nil {
		return nil, 0, err
	}
	return contacts, total, nil
}

func (r *ContactRepository) Save(ctx context.Context, contact *modelcustomer.Contact) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	return r.scopedContacts(ctx, contact.TenantUUID, contact.CustomerUUID).
		Where("uuid = ?", contact.UUID.String()).
		Select("display_name", "given_name", "family_name", "status", "roles", "tags", "updated_at").
		Updates(contact).Error
}

func (r *ContactRepository) FindIdentity(ctx context.Context, tenantUUID, channel, externalSubject string) (*modelcustomer.ContactIdentity, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var identity modelcustomer.ContactIdentity
	if err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND channel = ? AND external_subject = ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(channel), strings.TrimSpace(externalSubject)).
		First(&identity).Error; err != nil {
		return nil, err
	}
	return &identity, nil
}

func (r *ContactRepository) ListIdentities(ctx context.Context, tenantUUID, customerUUID, contactUUID string) ([]modelcustomer.ContactIdentity, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var identities []modelcustomer.ContactIdentity
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND customer_uuid = ? AND contact_uuid = ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(customerUUID), strings.TrimSpace(contactUUID)).Order("created_at DESC").Find(&identities).Error
	return identities, err
}

func (r *ContactRepository) CreateIdentity(ctx context.Context, identity *modelcustomer.ContactIdentity) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	return r.db.WithContext(ctx).Create(identity).Error
}

func (r *ContactRepository) scopedContacts(ctx context.Context, tenantUUID, customerUUID string) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&modelcustomer.Contact{}).
		Where("tenant_uuid = ? AND customer_uuid = ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(customerUUID))
}
