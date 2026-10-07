package customer

import (
	"context"
	"errors"
	"strings"
	"time"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AccountService struct {
	db   *gorm.DB
	repo *customerrepo.AccountRepository
}

func NewAccountService(db *gorm.DB) *AccountService {
	return &AccountService{db: db, repo: customerrepo.NewAccountRepository(db)}
}

type PrimaryContactInput struct {
	DisplayName string `json:"display_name"`
	GivenName   string `json:"given_name,omitempty"`
	FamilyName  string `json:"family_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
}

type ListAccountsInput struct {
	TenantUUID string
	Query      string
	Status     string
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

type CreateAccountInput struct {
	TenantUUID     string
	Type           string
	PrimaryContact *PrimaryContactInput
	Status         string
	PrimaryEmail   string
	PrimaryPhone   string
	DisplayName    string
	Nickname       string
	GivenName      string
	FamilyName     string
	AvatarURL      string
	Locale         string
	Timezone       string
	MemberSource   string
}

type ResolveExternalIdentityInput struct {
	TenantUUID, ProviderSubject, DisplayName, GivenName, FamilyName, Email, Phone string
}

func (s *AccountService) Overview(ctx context.Context, tenantUUID string) (customerrepo.OverviewRow, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return customerrepo.OverviewRow{}, err
	}
	return s.repo.Overview(ctx, tenantUUID)
}

func (s *AccountService) List(ctx context.Context, in ListAccountsInput) ([]customerrepo.AccountRow, int64, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(in.TenantUUID)
	if err != nil {
		return nil, 0, err
	}
	if status := strings.TrimSpace(in.Status); status != "" && !validCustomerStatus(status) {
		return nil, 0, errors.New("customer.invalid_status")
	}
	return s.repo.List(ctx, customerrepo.AccountListOptions{
		TenantUUID: tenantUUID,
		Query:      strings.TrimSpace(in.Query),
		Status:     strings.TrimSpace(in.Status),
		Page:       in.Page,
		PageSize:   in.PageSize,
		SortBy:     in.SortBy,
		SortOrder:  in.SortOrder,
	})
}

func (s *AccountService) Get(ctx context.Context, tenantUUID, customerUUID string) (customerrepo.AccountRow, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return customerrepo.AccountRow{}, err
	}
	if strings.TrimSpace(customerUUID) == "" {
		return customerrepo.AccountRow{}, errors.New("customer.uuid_required")
	}
	return s.repo.Get(ctx, tenantUUID, customerUUID)
}

func (s *AccountService) ListAuthIdentities(ctx context.Context, tenantUUID, customerUUID string) ([]customerrepo.CustomerIdentityRow, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	if _, err = s.repo.Get(ctx, tenantUUID, customerUUID); err != nil {
		return nil, err
	}
	return s.repo.ListAuthIdentities(ctx, tenantUUID, customerUUID)
}

func (s *AccountService) ListMemberships(ctx context.Context, tenantUUID, customerUUID string) ([]modelcustomer.TenantMembership, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	if _, err = s.repo.Get(ctx, tenantUUID, customerUUID); err != nil {
		return nil, err
	}
	return s.repo.ListMemberships(ctx, tenantUUID, customerUUID)
}

func (s *AccountService) ListLoginEvents(ctx context.Context, tenantUUID, customerUUID string, page, pageSize int) ([]modelcustomer.LoginEvent, int64, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, 0, err
	}
	if _, err = s.repo.Get(ctx, tenantUUID, customerUUID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListLoginEvents(ctx, tenantUUID, customerUUID, page, pageSize)
}

// ListMiniAppEntries is tenant-scoped configuration. It is intentionally not
// represented as a relation of a Customer because MiniAppEntry has no
// customer_uuid ownership field.
func (s *AccountService) ListMiniAppEntries(ctx context.Context, tenantUUID string, page, pageSize int) ([]modelcustomer.MiniAppEntry, int64, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListMiniAppEntries(ctx, tenantUUID, page, pageSize)
}

func (s *AccountService) Create(ctx context.Context, in CreateAccountInput) (customerrepo.AccountRow, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(in.TenantUUID)
	if err != nil {
		return customerrepo.AccountRow{}, err
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = modelcustomer.StatusActive
	}
	if !validCustomerStatus(status) {
		return customerrepo.AccountRow{}, errors.New("customer.invalid_status")
	}
	in.Type = strings.TrimSpace(in.Type)
	if in.Type == "" {
		in.Type = modelcustomer.AccountTypePerson
	}
	if in.Type != modelcustomer.AccountTypePerson && in.Type != modelcustomer.AccountTypeCompany {
		return customerrepo.AccountRow{}, errors.New("customer.type_required")
	}
	// display_name is a label, never an assertion of a person's legal name.
	if strings.TrimSpace(in.DisplayName) == "" {
		in.DisplayName = customerDisplayLabel(in.Nickname, in.GivenName, in.FamilyName, in.PrimaryEmail)
	}
	primary := in.PrimaryContact
	if primary == nil {
		if in.Type == modelcustomer.AccountTypeCompany {
			return customerrepo.AccountRow{}, errors.New("customer.primary_contact_required")
		}
		primary = &PrimaryContactInput{DisplayName: strings.TrimSpace(in.DisplayName), GivenName: strings.TrimSpace(in.GivenName), FamilyName: strings.TrimSpace(in.FamilyName), Email: strings.TrimSpace(in.PrimaryEmail), Phone: strings.TrimSpace(in.PrimaryPhone)}
	}
	copyPrimary := *primary
	primary = &copyPrimary
	if strings.TrimSpace(primary.DisplayName) == "" {
		primary.DisplayName = customerDisplayLabel("", primary.GivenName, primary.FamilyName, primary.Email)
	}
	if strings.TrimSpace(primary.DisplayName) == "" {
		return customerrepo.AccountRow{}, errors.New("customer.primary_contact_required")
	}
	account := &modelcustomer.Account{
		Type:         in.Type,
		Status:       status,
		PrimaryEmail: strings.TrimSpace(in.PrimaryEmail),
		PrimaryPhone: strings.TrimSpace(in.PrimaryPhone),
		DisplayName:  strings.TrimSpace(in.DisplayName),
		Nickname:     strings.TrimSpace(in.Nickname),
		GivenName:    strings.TrimSpace(in.GivenName),
		FamilyName:   strings.TrimSpace(in.FamilyName),
		AvatarURL:    strings.TrimSpace(in.AvatarURL),
		Locale:       strings.TrimSpace(in.Locale),
		Timezone:     strings.TrimSpace(in.Timezone),
		Metadata:     datatypes.JSON([]byte("{}")),
	}
	if account.DisplayName == "" && account.Nickname == "" && account.PrimaryEmail == "" && account.PrimaryPhone == "" {
		return customerrepo.AccountRow{}, errors.New("customer.identity_required")
	}
	source := strings.TrimSpace(in.MemberSource)
	if source == "" {
		source = "platform"
	}
	membership := &modelcustomer.TenantMembership{
		Status:   status,
		Source:   source,
		Roles:    datatypes.JSON([]byte("[]")),
		Scopes:   datatypes.JSON([]byte("[]")),
		Metadata: datatypes.JSON([]byte("{}")),
	}
	var primaryContactUUID string
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Contact creation requires an active parent; the requested final state is
		// applied before commit, so no intermediate state is externally visible.
		account.Status, membership.Status = modelcustomer.StatusActive, modelcustomer.StatusActive
		if err := customerrepo.NewAccountRepository(tx).CreateWithMembership(ctx, tenantUUID, account, membership); err != nil {
			return err
		}
		contact, err := NewContactService(tx).Create(ctx, CreateContactInput{TenantUUID: tenantUUID, CustomerUUID: account.UUID.String(), DisplayName: primary.DisplayName, GivenName: primary.GivenName, FamilyName: primary.FamilyName, Email: primary.Email, Phone: primary.Phone, Status: modelcustomer.ContactStatusActive, Roles: []string{modelcustomer.ContactRolePrimary}, CreationIntent: ContactCreationIntentExplicitCreate})
		if err != nil {
			return err
		}
		primaryContactUUID = contact.UUID.String()
		membership.PrimaryContactUUID = primaryContactUUID
		if err := tx.Model(membership).Update("primary_contact_uuid", primaryContactUUID).Error; err != nil {
			return err
		}
		if status != modelcustomer.StatusActive {
			if err := tx.Model(account).Update("status", status).Error; err != nil {
				return err
			}
			if err := tx.Model(membership).Update("status", status).Error; err != nil {
				return err
			}
		}
		account.Status, membership.Status = status, status
		return nil
	}); err != nil {
		return customerrepo.AccountRow{}, err
	}
	return customerrepo.AccountRow{
		UUID: account.UUID.String(), Type: account.Type, PrimaryContactUUID: primaryContactUUID, Status: account.Status, PrimaryEmail: account.PrimaryEmail,
		PrimaryPhone: account.PrimaryPhone, DisplayName: account.DisplayName, Nickname: account.Nickname,
		GivenName: account.GivenName, FamilyName: account.FamilyName, AvatarURL: account.AvatarURL,
		Locale: account.Locale, Timezone: account.Timezone, MemberStatus: membership.Status,
		MemberSource: membership.Source, MembershipUUID: membership.UUID.String(),
		CreatedAt: account.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: account.UpdatedAt.Format(time.RFC3339Nano),
	}, nil
}

func (s *AccountService) ResolveExternalIdentity(ctx context.Context, in ResolveExternalIdentityInput) (customerrepo.ExternalIdentityResolution, error) {
	return s.resolveExternalIdentity(ctx, in, false)
}

func (s *AccountService) resolveVerifiedExternalIdentity(ctx context.Context, in ResolveExternalIdentityInput) (customerrepo.ExternalIdentityResolution, error) {
	return s.resolveExternalIdentity(ctx, in, true)
}

func (s *AccountService) resolveExternalIdentity(ctx context.Context, in ResolveExternalIdentityInput, verifiedByCore bool) (customerrepo.ExternalIdentityResolution, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(in.TenantUUID)
	if err != nil {
		return customerrepo.ExternalIdentityResolution{}, err
	}
	pluginID, err := externalIdentityPluginID(ctx)
	if err != nil {
		return customerrepo.ExternalIdentityResolution{}, err
	}
	providerKey, err := customerrepo.ExternalIdentityProviderKey(pluginID)
	if err != nil {
		return customerrepo.ExternalIdentityResolution{}, err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		in.DisplayName = customerDisplayLabel("", in.GivenName, in.FamilyName, in.Email)
	}
	return s.repo.ResolveOrCreateExternalIdentity(ctx, customerrepo.ExternalIdentityInput{TenantUUID: tenantUUID, ProviderKey: providerKey, ProviderPluginID: pluginID, ProviderSubject: strings.TrimSpace(in.ProviderSubject), DisplayName: strings.TrimSpace(in.DisplayName), GivenName: strings.TrimSpace(in.GivenName), FamilyName: strings.TrimSpace(in.FamilyName), Email: strings.TrimSpace(in.Email), Phone: strings.TrimSpace(in.Phone), VerifiedByCore: verifiedByCore})
}

func externalIdentityPluginID(ctx context.Context) (string, error) {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || !containsAudience(claims.Audience, "powerx:api") {
		return "", customerrepo.ErrExternalIdentityServiceActorInvalid
	}
	pluginID := strings.TrimSpace(claims.PluginID)
	if pluginID == "" {
		return "", customerrepo.ErrExternalIdentityRequired
	}
	return pluginID, nil
}

func containsAudience(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func (s *AccountService) UpdateStatus(ctx context.Context, tenantUUID string, customerUUID string, status string) error {
	canonicalTenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return err
	}
	customerUUID = strings.TrimSpace(customerUUID)
	if customerUUID == "" {
		return errors.New("customer.uuid_required")
	}
	status = strings.TrimSpace(status)
	if !validCustomerStatus(status) {
		return errors.New("customer.invalid_status")
	}
	return s.repo.UpdateTenantStatus(ctx, canonicalTenantUUID, customerUUID, status)
}

func validCustomerStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case modelcustomer.StatusActive,
		modelcustomer.StatusPending,
		modelcustomer.StatusSuspended,
		modelcustomer.StatusDisabled,
		modelcustomer.StatusExpired,
		modelcustomer.StatusDeleted:
		return true
	default:
		return false
	}
}

func customerDisplayLabel(nickname, given, family, email string) string {
	if value := strings.TrimSpace(nickname); value != "" {
		return value
	}
	if value := strings.TrimSpace(strings.TrimSpace(given) + " " + strings.TrimSpace(family)); value != "" {
		return value
	}
	return strings.TrimSpace(email)
}
