package customer

import (
	"context"
	"errors"
	"strings"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AccountService struct {
	repo *customerrepo.AccountRepository
}

func NewAccountService(db *gorm.DB) *AccountService {
	return &AccountService{repo: customerrepo.NewAccountRepository(db)}
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
	TenantUUID   string
	Status       string
	PrimaryEmail string
	PrimaryPhone string
	DisplayName  string
	Nickname     string
	GivenName    string
	FamilyName   string
	AvatarURL    string
	Locale       string
	Timezone     string
	MemberSource string
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
	account := &modelcustomer.Account{
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
	if err := s.repo.CreateWithMembership(ctx, tenantUUID, account, membership); err != nil {
		return customerrepo.AccountRow{}, err
	}
	rows, _, err := s.repo.List(ctx, customerrepo.AccountListOptions{
		TenantUUID: tenantUUID,
		Query:      account.UUID.String(),
		Page:       1,
		PageSize:   1,
	})
	if err != nil {
		return customerrepo.AccountRow{}, err
	}
	if len(rows) == 0 {
		return customerrepo.AccountRow{}, gorm.ErrRecordNotFound
	}
	return rows[0], nil
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
