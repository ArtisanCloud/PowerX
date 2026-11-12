package customer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	CustomerMembershipsServiceReadCapabilityID = "com.corex.customer.memberships.service_read"
	CustomerMembershipsCoreEndpoint            = "core://customer/memberships"
)

var (
	ErrVerificationInvalidArgument       = errors.New("customer.verification_invalid_argument")
	ErrVerificationUnauthorized          = errors.New("customer.verification_unauthorized")
	ErrVerificationForbidden             = errors.New("customer.verification_forbidden")
	ErrVerificationNotFound              = errors.New("customer.verification_not_found")
	ErrVerificationCustomerInactive      = errors.New("customer.verification_customer_inactive")
	ErrVerificationMembershipInactive    = errors.New("customer.verification_membership_inactive")
	ErrVerificationMembershipExpired     = errors.New("customer.verification_membership_expired")
	ErrVerificationPrimaryContactInvalid = errors.New("customer.verification_primary_contact_invalid")
	ErrVerificationUnavailable           = errors.New("customer.verification_unavailable")
)

// CustomerVerification 是一次核验结果，不是客户登录凭证或后续交易授权。
type CustomerVerification struct {
	SchemaVersion      int        `json:"schema_version"`
	TenantUUID         string     `json:"tenant_uuid"`
	CustomerUUID       string     `json:"customer_uuid"`
	MembershipUUID     string     `json:"membership_uuid"`
	PrimaryContactUUID string     `json:"primary_contact_uuid"`
	Status             string     `json:"status"`
	ExpiresAt          *time.Time `json:"expires_at"`
	CheckedAt          time.Time  `json:"checked_at"`
	Revision           string     `json:"revision"`
}

type MembershipVerificationService struct {
	db   *gorm.DB
	repo *customerrepo.AccountRepository
	now  func() time.Time
}

func NewMembershipVerificationService(db *gorm.DB) *MembershipVerificationService {
	return &MembershipVerificationService{db: db, repo: customerrepo.NewAccountRepository(db), now: time.Now}
}

// VerifyCustomer 仅接受目标客户；租户和调用主体均来自认证上下文，权限实时回查。
func (s *MembershipVerificationService) VerifyCustomer(ctx context.Context, customerUUID string) (CustomerVerification, error) {
	if s == nil || s.db == nil || s.repo == nil {
		return CustomerVerification{}, ErrVerificationUnavailable
	}
	if !capabilityregistry.ServiceCredential(ctx) {
		return CustomerVerification{}, ErrVerificationUnauthorized
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if err != nil {
		return CustomerVerification{}, ErrVerificationUnauthorized
	}
	access, err := capabilityregistry.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err != nil {
		var denied *capabilityregistry.DirectGrantError
		if errors.As(err, &denied) {
			switch denied.Status {
			case 401:
				return CustomerVerification{}, ErrVerificationUnauthorized
			case 403:
				return CustomerVerification{}, ErrVerificationForbidden
			}
		}
		return CustomerVerification{}, ErrVerificationUnavailable
	}
	if err := access.Require(CustomerMembershipsServiceReadCapabilityID); err != nil {
		return CustomerVerification{}, ErrVerificationForbidden
	}
	if !verificationUUID(customerUUID) {
		return CustomerVerification{}, ErrVerificationInvalidArgument
	}
	row, err := s.repo.MembershipVerification(ctx, tenantUUID, customerUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CustomerVerification{}, ErrVerificationNotFound
	}
	if err != nil {
		return CustomerVerification{}, ErrVerificationUnavailable
	}
	checkedAt := s.now().UTC()
	if row.TenantUUID != tenantUUID || row.CustomerUUID != customerUUID || !verificationUUID(row.MembershipUUID) {
		return CustomerVerification{}, ErrVerificationUnavailable
	}
	if row.AccountStatus != modelcustomer.StatusActive {
		return CustomerVerification{}, ErrVerificationCustomerInactive
	}
	if row.MembershipStatus != modelcustomer.StatusActive {
		return CustomerVerification{}, ErrVerificationMembershipInactive
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(checkedAt) {
		return CustomerVerification{}, ErrVerificationMembershipExpired
	}
	if !verificationUUID(row.PrimaryContactUUID) || row.ContactUUID != row.PrimaryContactUUID || row.ContactStatus != modelcustomer.ContactStatusActive {
		return CustomerVerification{}, ErrVerificationPrimaryContactInvalid
	}
	row.AccountUpdatedAt, row.MembershipUpdatedAt = row.AccountUpdatedAt.UTC(), row.MembershipUpdatedAt.UTC()
	if row.ContactUpdatedAt != nil {
		v := row.ContactUpdatedAt.UTC()
		row.ContactUpdatedAt = &v
	}
	if row.ExpiresAt != nil {
		v := row.ExpiresAt.UTC()
		row.ExpiresAt = &v
	}
	// checked_at 不进入摘要：相同快照重复核验得到同一个 revision。
	raw, err := json.Marshal(row)
	if err != nil {
		return CustomerVerification{}, ErrVerificationUnavailable
	}
	digest := sha256.Sum256(raw)
	return CustomerVerification{
		SchemaVersion: 1, TenantUUID: tenantUUID, CustomerUUID: customerUUID,
		MembershipUUID: row.MembershipUUID, PrimaryContactUUID: row.PrimaryContactUUID,
		Status: modelcustomer.StatusActive, ExpiresAt: row.ExpiresAt, CheckedAt: checkedAt,
		Revision: "sha256:" + hex.EncodeToString(digest[:]),
	}, nil
}

func verificationUUID(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}
