package customer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"gorm.io/gorm"
)

const CustomerMembershipsDelegatedReadCapabilityID = "com.corex.customer.memberships.delegated_read"

var (
	ErrCustomerUnauthorized       = errors.New("customer.unauthorized")
	ErrCustomerForbidden          = errors.New("customer.forbidden")
	ErrCustomerMembershipNotFound = errors.New("customer.membership_not_found")
	ErrCustomerMembershipInactive = errors.New("customer.membership_inactive")
	ErrCustomerUpstreamDependency = errors.New("customer.upstream_dependency")
)

type Membership struct {
	TenantUUID     string     `json:"tenant_uuid"`
	CustomerUUID   string     `json:"customer_uuid"`
	MembershipUUID string     `json:"membership_uuid"`
	Status         string     `json:"status"`
	Roles          []string   `json:"roles"`
	Scopes         []string   `json:"scopes"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type MembershipService struct {
	db   *gorm.DB
	repo *customerrepo.AccountRepository
}

func NewMembershipService(db *gorm.DB) *MembershipService {
	return &MembershipService{db: db, repo: customerrepo.NewAccountRepository(db)}
}

func (s *MembershipService) ResolveCurrent(ctx context.Context, tenantUUID, customerUUID string) (Membership, error) {
	if s == nil || s.repo == nil {
		return Membership{}, ErrCustomerUpstreamDependency
	}
	row, err := s.repo.CurrentMembership(ctx, strings.TrimSpace(tenantUUID), strings.TrimSpace(customerUUID))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Membership{}, ErrCustomerMembershipNotFound
	}
	if err != nil {
		return Membership{}, ErrCustomerUpstreamDependency
	}
	if row.Status != modelcustomer.StatusActive || row.AccountStatus != modelcustomer.StatusActive || (row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now())) {
		return Membership{}, ErrCustomerMembershipInactive
	}
	var roles, scopes []string
	if json.Unmarshal(row.Roles, &roles) != nil || json.Unmarshal(row.Scopes, &scopes) != nil {
		return Membership{}, ErrCustomerUpstreamDependency
	}
	return Membership{TenantUUID: row.TenantUUID, CustomerUUID: row.CustomerUUID, MembershipUUID: row.MembershipUUID, Status: row.Status, Roles: roles, Scopes: scopes, ExpiresAt: row.ExpiresAt}, nil
}

// AuthorizeDelegatedActor proves the STS caller is registered and actually
// granted the single, tenant-scoped customer-membership capability.
func (s *MembershipService) AuthorizeDelegatedActor(ctx context.Context, tenantUUID, pluginID string) error {
	return s.AuthorizeDelegatedActorForCapability(ctx, tenantUUID, pluginID, CustomerMembershipsDelegatedReadCapabilityID)
}

// AuthorizeDelegatedActorForCapability verifies publication, tenant registration
// and the concrete STS credential grant for an exact delegated capability.
func (s *MembershipService) AuthorizeDelegatedActorForCapability(ctx context.Context, tenantUUID, pluginID, requiredCapabilityID string) error {
	if s == nil || s.db == nil || strings.TrimSpace(pluginID) == "" {
		return ErrCustomerForbidden
	}
	requiredCapabilityID = strings.TrimSpace(requiredCapabilityID)
	if requiredCapabilityID == "" {
		return ErrCustomerForbidden
	}
	var capability capmodels.CapabilityRecord
	if err := s.db.WithContext(ctx).Where("capability_id = ? AND status = ?", requiredCapabilityID, "published").First(&capability).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCustomerForbidden
		}
		return ErrCustomerUpstreamDependency
	}
	var registration capmodels.CapabilityRegistration
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND capability_id = ? AND status = ?", tenantUUID, requiredCapabilityID, "published").First(&registration).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCustomerForbidden
		}
		return ErrCustomerUpstreamDependency
	}
	var credential settingmodel.PluginInstanceConfig
	err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, pluginID, settingrepo.KeyClientCredentials, true).First(&credential).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrCustomerForbidden
	}
	if err != nil {
		return ErrCustomerUpstreamDependency
	}
	var payload struct {
		AllowedCapabilities []string `json:"allowed_capabilities"`
	}
	if json.Unmarshal(credential.ValueJSON, &payload) != nil {
		return ErrCustomerUpstreamDependency
	}
	for _, grantedCapabilityID := range payload.AllowedCapabilities {
		if strings.TrimSpace(grantedCapabilityID) == requiredCapabilityID {
			return nil
		}
	}
	return ErrCustomerForbidden
}
