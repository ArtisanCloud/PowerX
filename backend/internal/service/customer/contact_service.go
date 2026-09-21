package customer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	contactrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrContactInvalidArgument            = errors.New("contact.invalid_argument")
	ErrContactNotFound                   = errors.New("contact.not_found")
	ErrContactCustomerMismatch           = errors.New("contact.customer_mismatch")
	ErrContactCustomerMembershipInactive = errors.New("contact.customer_membership_inactive")
	ErrContactIdentityNotFound           = errors.New("contact.identity_not_found")
	ErrContactIdentityConflict           = errors.New("contact.identity_conflict")
)

const (
	ContactCreationIntentExplicitCreate    = "explicit_create"
	ContactCreationIntentExplicitTemporary = "explicit_temporary"
)

var contactTagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type ContactService struct {
	db       *gorm.DB
	accounts *contactrepo.AccountRepository
}

func NewContactService(db *gorm.DB) *ContactService {
	return &ContactService{db: db, accounts: contactrepo.NewAccountRepository(db)}
}

type CreateContactInput struct {
	TenantUUID     string
	CustomerUUID   string
	DisplayName    string
	GivenName      string
	FamilyName     string
	Status         string
	Roles          []string
	Tags           []string
	CreationIntent string
}

type UpdateContactInput struct {
	TenantUUID   string
	CustomerUUID string
	ContactUUID  string
	DisplayName  *string
	GivenName    *string
	FamilyName   *string
	Status       *string
	Roles        *[]string
	Tags         *[]string
}

type ListContactsInput struct {
	TenantUUID   string
	CustomerUUID string
	Query        string
	Status       string
	Page         int
	PageSize     int
}

type ContactPage struct {
	Items    []modelcustomer.Contact
	Total    int64
	Page     int
	PageSize int
}

type ResolveContactIdentityInput struct {
	TenantUUID      string
	CustomerUUID    string
	Channel         string
	ExternalSubject string
}

type ContactIdentityResolution struct {
	Contact  *modelcustomer.Contact
	Identity *modelcustomer.ContactIdentity
}

type BindContactIdentityInput struct {
	TenantUUID      string
	CustomerUUID    string
	ContactUUID     string
	Channel         string
	ExternalSubject string
}

func (s *ContactService) Create(ctx context.Context, input CreateContactInput) (*modelcustomer.Contact, error) {
	if s == nil || s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	tenantUUID, customerUUID, err := canonicalContactScope(input.TenantUUID, input.CustomerUUID)
	if err != nil {
		return nil, err
	}
	roles, err := normalizeContactRoles(input.Roles)
	if err != nil {
		return nil, err
	}
	tags, err := normalizeContactTags(input.Tags)
	if err != nil {
		return nil, err
	}
	status, intent, err := normalizeContactCreateState(input.Status, input.CreationIntent)
	if err != nil {
		return nil, err
	}
	contact := &modelcustomer.Contact{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID,
		DisplayName: strings.TrimSpace(input.DisplayName), GivenName: strings.TrimSpace(input.GivenName), FamilyName: strings.TrimSpace(input.FamilyName),
		Status: status, Roles: marshalStringArray(roles), Tags: marshalStringArray(tags),
		Metadata: datatypes.JSON([]byte(fmt.Sprintf(`{"creation_intent":%q}`, intent))),
	}
	if contact.DisplayName == "" || len(contact.DisplayName) > 128 || len(contact.GivenName) > 128 || len(contact.FamilyName) > 128 {
		return nil, ErrContactInvalidArgument
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureActiveCustomer(ctx, tx, tenantUUID, customerUUID); err != nil {
			return err
		}
		return contactrepo.NewContactRepository(tx).Create(ctx, contact)
	}); err != nil {
		return nil, err
	}
	return contact, nil
}

func (s *ContactService) Get(ctx context.Context, tenantUUID, customerUUID, contactUUID string) (*modelcustomer.Contact, error) {
	if s == nil || s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	tenantUUID, customerUUID, err := canonicalContactScope(tenantUUID, customerUUID)
	if err != nil {
		return nil, err
	}
	if _, err := canonicalUUID(contactUUID); err != nil {
		return nil, err
	}
	if err := ensureActiveCustomer(ctx, s.db, tenantUUID, customerUUID); err != nil {
		return nil, err
	}
	return getScopedContact(ctx, contactrepo.NewContactRepository(s.db), tenantUUID, customerUUID, contactUUID)
}

func (s *ContactService) ListIdentities(ctx context.Context, tenantUUID, customerUUID, contactUUID string) ([]modelcustomer.ContactIdentity, error) {
	tenantUUID, customerUUID, err := canonicalContactScope(tenantUUID, customerUUID)
	if err != nil {
		return nil, err
	}
	if _, err := canonicalUUID(contactUUID); err != nil {
		return nil, err
	}
	if err := ensureActiveCustomer(ctx, s.db, tenantUUID, customerUUID); err != nil {
		return nil, err
	}
	if _, err := getScopedContact(ctx, contactrepo.NewContactRepository(s.db), tenantUUID, customerUUID, contactUUID); err != nil {
		return nil, err
	}
	return contactrepo.NewContactRepository(s.db).ListIdentities(ctx, tenantUUID, customerUUID, contactUUID)
}

func (s *ContactService) ListByCustomer(ctx context.Context, input ListContactsInput) (ContactPage, error) {
	if s == nil || s.db == nil {
		return ContactPage{}, gorm.ErrInvalidDB
	}
	tenantUUID, customerUUID, err := canonicalContactScope(input.TenantUUID, input.CustomerUUID)
	if err != nil {
		return ContactPage{}, err
	}
	if status := strings.TrimSpace(input.Status); status != "" && !validContactStatus(status) {
		return ContactPage{}, ErrContactInvalidArgument
	}
	if err := ensureActiveCustomer(ctx, s.db, tenantUUID, customerUUID); err != nil {
		return ContactPage{}, err
	}
	items, total, err := contactrepo.NewContactRepository(s.db).List(ctx, contactrepo.ContactListOptions{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, Query: input.Query, Status: input.Status, Page: input.Page, PageSize: input.PageSize,
	})
	if err != nil {
		return ContactPage{}, err
	}
	page, pageSize := input.Page, input.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return ContactPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ContactService) Update(ctx context.Context, input UpdateContactInput) (*modelcustomer.Contact, error) {
	if s == nil || s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	tenantUUID, customerUUID, err := canonicalContactScope(input.TenantUUID, input.CustomerUUID)
	if err != nil {
		return nil, err
	}
	if _, err := canonicalUUID(input.ContactUUID); err != nil {
		return nil, err
	}
	var updated *modelcustomer.Contact
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureActiveCustomer(ctx, tx, tenantUUID, customerUUID); err != nil {
			return err
		}
		repo := contactrepo.NewContactRepository(tx)
		contact, err := getScopedContact(ctx, repo, tenantUUID, customerUUID, input.ContactUUID)
		if err != nil {
			return err
		}
		if input.DisplayName != nil {
			contact.DisplayName = strings.TrimSpace(*input.DisplayName)
			if contact.DisplayName == "" || len(contact.DisplayName) > 128 {
				return ErrContactInvalidArgument
			}
		}
		if input.GivenName != nil {
			contact.GivenName = strings.TrimSpace(*input.GivenName)
			if len(contact.GivenName) > 128 {
				return ErrContactInvalidArgument
			}
		}
		if input.FamilyName != nil {
			contact.FamilyName = strings.TrimSpace(*input.FamilyName)
			if len(contact.FamilyName) > 128 {
				return ErrContactInvalidArgument
			}
		}
		if input.Status != nil {
			status := strings.TrimSpace(*input.Status)
			if !validContactStatus(status) {
				return ErrContactInvalidArgument
			}
			contact.Status = status
		}
		if input.Roles != nil {
			roles, err := normalizeContactRoles(*input.Roles)
			if err != nil {
				return err
			}
			contact.Roles = marshalStringArray(roles)
		}
		if input.Tags != nil {
			tags, err := normalizeContactTags(*input.Tags)
			if err != nil {
				return err
			}
			contact.Tags = marshalStringArray(tags)
		}
		if err := repo.Save(ctx, contact); err != nil {
			return err
		}
		updated = contact
		return nil
	})
	return updated, err
}

func (s *ContactService) ResolveIdentity(ctx context.Context, input ResolveContactIdentityInput) (*ContactIdentityResolution, error) {
	if s == nil || s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	tenantUUID, customerUUID, err := canonicalContactScope(input.TenantUUID, input.CustomerUUID)
	if err != nil {
		return nil, err
	}
	channel, subject, err := canonicalIdentity(input.Channel, input.ExternalSubject)
	if err != nil {
		return nil, err
	}
	if err := ensureActiveCustomer(ctx, s.db, tenantUUID, customerUUID); err != nil {
		return nil, err
	}
	repo := contactrepo.NewContactRepository(s.db)
	identity, err := repo.FindIdentity(ctx, tenantUUID, channel, subject)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrContactIdentityNotFound
	}
	if err != nil {
		return nil, err
	}
	if identity.CustomerUUID != customerUUID {
		return nil, ErrContactCustomerMismatch
	}
	contact, err := repo.Get(ctx, tenantUUID, customerUUID, identity.ContactUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrContactIdentityNotFound
	}
	if err != nil {
		return nil, err
	}
	if contact.Status != modelcustomer.ContactStatusActive || identity.Status != modelcustomer.ContactIdentityStatusActive {
		return nil, ErrContactIdentityNotFound
	}
	return &ContactIdentityResolution{Contact: contact, Identity: identity}, nil
}

func (s *ContactService) BindIdentity(ctx context.Context, input BindContactIdentityInput) (*modelcustomer.ContactIdentity, error) {
	if s == nil || s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	tenantUUID, customerUUID, err := canonicalContactScope(input.TenantUUID, input.CustomerUUID)
	if err != nil {
		return nil, err
	}
	if _, err := canonicalUUID(input.ContactUUID); err != nil {
		return nil, err
	}
	channel, subject, err := canonicalIdentity(input.Channel, input.ExternalSubject)
	if err != nil {
		return nil, err
	}
	identity := &modelcustomer.ContactIdentity{TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: strings.TrimSpace(input.ContactUUID), Channel: channel, ExternalSubject: subject, Status: modelcustomer.ContactIdentityStatusActive, Metadata: datatypes.JSON([]byte("{}"))}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureActiveCustomer(ctx, tx, tenantUUID, customerUUID); err != nil {
			return err
		}
		repo := contactrepo.NewContactRepository(tx)
		if _, err := getScopedContact(ctx, repo, tenantUUID, customerUUID, input.ContactUUID); err != nil {
			return err
		}
		if existing, err := repo.FindIdentity(ctx, tenantUUID, channel, subject); err == nil {
			if existing.CustomerUUID != customerUUID || existing.ContactUUID != input.ContactUUID {
				return ErrContactIdentityConflict
			}
			return ErrContactIdentityConflict
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return repo.CreateIdentity(ctx, identity)
	})
	if isUniqueViolation(err) {
		return nil, ErrContactIdentityConflict
	}
	if err != nil {
		return nil, err
	}
	return identity, nil
}

func getScopedContact(ctx context.Context, repo *contactrepo.ContactRepository, tenantUUID, customerUUID, contactUUID string) (*modelcustomer.Contact, error) {
	contact, err := repo.Get(ctx, tenantUUID, customerUUID, contactUUID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return contact, err
	}
	if _, tenantErr := repo.FindByTenant(ctx, tenantUUID, contactUUID); tenantErr == nil {
		return nil, ErrContactCustomerMismatch
	} else if !errors.Is(tenantErr, gorm.ErrRecordNotFound) {
		return nil, tenantErr
	}
	return nil, ErrContactNotFound
}

func ensureActiveCustomer(ctx context.Context, db *gorm.DB, tenantUUID, customerUUID string) error {
	membership, err := contactrepo.NewAccountRepository(db).CurrentMembership(ctx, tenantUUID, customerUUID)
	if err != nil || membership.Status != modelcustomer.StatusActive || membership.AccountStatus != modelcustomer.StatusActive {
		return ErrContactCustomerMembershipInactive
	}
	return nil
}

func canonicalContactScope(tenantUUID, customerUUID string) (string, string, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return "", "", ErrContactInvalidArgument
	}
	customerUUID, err = canonicalUUID(customerUUID)
	if err != nil {
		return "", "", err
	}
	return tenantUUID, customerUUID, nil
}

func canonicalUUID(raw string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", ErrContactInvalidArgument
	}
	return parsed.String(), nil
}

func canonicalIdentity(channel, subject string) (string, string, error) {
	channel, subject = strings.ToLower(strings.TrimSpace(channel)), strings.TrimSpace(subject)
	if channel == "" || len(channel) > 64 || subject == "" || len(subject) > 255 {
		return "", "", ErrContactInvalidArgument
	}
	return channel, subject, nil
}

func normalizeContactCreateState(status, intent string) (string, string, error) {
	status, intent = strings.TrimSpace(status), strings.TrimSpace(intent)
	if status == "" {
		status = modelcustomer.ContactStatusActive
	}
	if status == modelcustomer.ContactStatusTemporary {
		if intent != ContactCreationIntentExplicitTemporary {
			return "", "", ErrContactInvalidArgument
		}
	} else if intent != ContactCreationIntentExplicitCreate || !validContactStatus(status) {
		return "", "", ErrContactInvalidArgument
	}
	return status, intent, nil
}

func validContactStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case modelcustomer.ContactStatusActive, modelcustomer.ContactStatusInactive, modelcustomer.ContactStatusTemporary:
		return true
	default:
		return false
	}
}

func normalizeContactRoles(roles []string) ([]string, error) {
	seen, normalized := map[string]struct{}{}, make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		switch role {
		case modelcustomer.ContactRolePrimary, modelcustomer.ContactRoleLegalRepresentative:
		default:
			return nil, ErrContactInvalidArgument
		}
		if _, ok := seen[role]; !ok {
			seen[role] = struct{}{}
			normalized = append(normalized, role)
		}
	}
	return normalized, nil
}

func normalizeContactTags(tags []string) ([]string, error) {
	if len(tags) > 20 {
		return nil, ErrContactInvalidArgument
	}
	seen, normalized, total := map[string]struct{}{}, make([]string, 0, len(tags)), 0
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if !contactTagPattern.MatchString(tag) {
			return nil, ErrContactInvalidArgument
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		total += len(tag)
		if total > 1024 {
			return nil, ErrContactInvalidArgument
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized, nil
}

func marshalStringArray(values []string) datatypes.JSON {
	raw, _ := json.Marshal(values)
	return datatypes.JSON(raw)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "unique") || strings.Contains(value, "duplicate")
}
