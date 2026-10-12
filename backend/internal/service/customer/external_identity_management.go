package customer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	capsvc "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	CustomerExternalIdentitiesReadCapabilityID   = "com.corex.customer.external_identities.service_read"
	CustomerExternalIdentitiesManageCapabilityID = "com.corex.customer.external_identities.service_manage"
)

var ErrExternalIdentityConflict = errors.New("customer.external_identity_conflict")

// ExternalIdentityItem excludes password hashes and unrelated identity providers.
type ExternalIdentityItem struct {
	IdentityUUID       string `json:"identity_uuid"`
	CustomerUUID       string `json:"customer_uuid"`
	ProviderSubject    string `json:"provider_subject"`
	Status             string `json:"status"`
	Type               string `json:"type"`
	PrimaryContactUUID string `json:"primary_contact_uuid,omitempty"`
}

type ExternalCustomerProfile struct {
	Type           string               `json:"type,omitempty"`
	DisplayName    string               `json:"display_name,omitempty"`
	Nickname       string               `json:"nickname,omitempty"`
	GivenName      string               `json:"given_name,omitempty"`
	FamilyName     string               `json:"family_name,omitempty"`
	PrimaryEmail   string               `json:"primary_email,omitempty"`
	PrimaryPhone   string               `json:"primary_phone,omitempty"`
	PrimaryContact *PrimaryContactInput `json:"primary_contact,omitempty"`
}
type ExternalIdentityRequest struct {
	Operation       string                   `json:"operation"`
	ProviderSubject string                   `json:"provider_subject,omitempty"`
	CustomerUUID    string                   `json:"customer_uuid,omitempty"`
	Customer        *ExternalCustomerProfile `json:"customer,omitempty"`
	Page            int                      `json:"page,omitempty"`
	PageSize        int                      `json:"page_size,omitempty"`
}

func (s *AccountService) externalIdentityActor(ctx context.Context, capability string) (string, string, error) {
	tenant, err := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if err != nil {
		return "", "", repo.ErrExternalIdentityServiceActorInvalid
	}
	access, err := capsvc.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err != nil {
		return "", "", err
	}
	if err = access.Require(capability); err != nil {
		return "", "", err
	}
	if hash := reqctx.AuthenticatedAPIKeyHash(ctx); hash != "" {
		key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).FindActiveByHash(ctx, tenant, hash)
		if err != nil {
			return "", "", err
		}
		if key == nil {
			return "", "", repo.ErrExternalIdentityServiceActorInvalid
		}
		action := "read"
		suffix := "service_read"
		if capability == CustomerExternalIdentitiesManageCapabilityID {
			action = "manage"
			suffix = "service_manage"
		}
		var grants []gw.IntegrationGatewayAPIKeyPermission
		err = s.db.WithContext(ctx).Where("api_key_uuid = ? AND scope = ? AND action = ? AND resource_type = ? AND resource_pattern = ? AND effect = ?", key.UUID, "_scope.customer.external_identities."+suffix, action, "capability", "customer_external_identities_"+suffix, "allow").Find(&grants).Error
		if err != nil {
			return "", "", err
		}
		plugin := ""
		for _, g := range grants {
			p := strings.TrimSpace(g.PluginID)
			if p == "" || (plugin != "" && plugin != p) {
				return "", "", repo.ErrExternalIdentityServiceActorInvalid
			}
			plugin = p
		}
		if plugin == "" {
			return "", "", repo.ErrExternalIdentityServiceActorInvalid
		}
		return tenant, plugin, nil
	}
	plugin, err := externalIdentityPluginID(ctx)
	return tenant, plugin, err
}

func validateIdentityManagementRequest(in ExternalIdentityRequest, write bool) error {
	switch in.Operation {
	case "lookup", "bind", "create_and_bind":
		parts := strings.SplitN(in.ProviderSubject, ":", 4)
		if len(parts) != 4 || len(in.ProviderSubject) > 255 || strings.IndexFunc(in.ProviderSubject, unicode.IsControl) >= 0 {
			return ErrCustomerAccountInvalidArgument
		}
		for _, p := range parts {
			if strings.TrimSpace(p) == "" || p != strings.TrimSpace(p) {
				return ErrCustomerAccountInvalidArgument
			}
		}
	case "list_by_customer":
	default:
		return ErrCustomerAccountInvalidArgument
	}
	if write != (in.Operation == "bind" || in.Operation == "create_and_bind") {
		return ErrCustomerAccountInvalidArgument
	}
	if in.Operation == "bind" || in.Operation == "list_by_customer" {
		id, e := uuid.Parse(in.CustomerUUID)
		if e != nil || id == uuid.Nil {
			return ErrCustomerAccountInvalidArgument
		}
	} else if in.CustomerUUID != "" {
		return ErrCustomerAccountInvalidArgument
	}
	if (in.Operation == "create_and_bind") != (in.Customer != nil) {
		return ErrCustomerAccountInvalidArgument
	}
	if in.Operation != "list_by_customer" && (in.Page != 0 || in.PageSize != 0) {
		return ErrCustomerAccountInvalidArgument
	}
	if in.Operation == "list_by_customer" && (in.ProviderSubject != "" || in.Page < 0 || in.PageSize < 0 || in.PageSize > 100) {
		return ErrCustomerAccountInvalidArgument
	}
	if in.Customer != nil {
		p := in.Customer
		if p.Type != "" && p.Type != "person" && p.Type != "company" {
			return ErrCustomerAccountInvalidArgument
		}
		label := strings.TrimSpace(p.DisplayName)
		if label == "" {
			label = customerDisplayLabel(p.Nickname, p.GivenName, p.FamilyName, p.PrimaryEmail)
		}
		if _, err := validateAccountPatch(AccountProfilePatch{DisplayName: &label, Nickname: &p.Nickname, GivenName: &p.GivenName, FamilyName: &p.FamilyName, PrimaryEmail: &p.PrimaryEmail, PrimaryPhone: &p.PrimaryPhone}); err != nil {
			return err
		}
		if p.Type == "company" && p.PrimaryContact == nil {
			return ErrCustomerAccountInvalidArgument
		}
	}
	return nil
}

func identityManagementItem(db *gorm.DB, tenant string, identity model.AuthIdentity) (ExternalIdentityItem, error) {
	var membership model.TenantMembership
	if err := db.Where("tenant_uuid = ? AND customer_uuid = ?", tenant, identity.CustomerUUID).First(&membership).Error; err != nil {
		return ExternalIdentityItem{}, err
	}
	var account model.Account
	if err := db.Where("uuid = ?", identity.CustomerUUID).First(&account).Error; err != nil {
		return ExternalIdentityItem{}, err
	}
	return ExternalIdentityItem{IdentityUUID: identity.UUID.String(), CustomerUUID: identity.CustomerUUID, ProviderSubject: identity.ProviderSubject, Status: identity.Status, Type: account.Type, PrimaryContactUUID: membership.PrimaryContactUUID}, nil
}

// ManageExternalIdentity exposes read-only inspection and explicit writes separately.
func (s *AccountService) ManageExternalIdentity(ctx context.Context, capability string, in ExternalIdentityRequest) (map[string]any, error) {
	write := capability == CustomerExternalIdentitiesManageCapabilityID
	if capability != CustomerExternalIdentitiesReadCapabilityID && !write {
		return nil, ErrCustomerAccountInvalidArgument
	}
	if err := validateIdentityManagementRequest(in, write); err != nil {
		return nil, err
	}
	tenant, plugin, err := s.externalIdentityActor(ctx, capability)
	if err != nil {
		return nil, err
	}
	provider, err := repo.ExternalIdentityProviderKey(plugin)
	if err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	if in.Operation == "lookup" {
		var identity model.AuthIdentity
		err := db.Where("provider = ? AND provider_subject = ?", provider, in.ProviderSubject).First(&identity).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]any{"found": false}, nil
		}
		if err != nil {
			return nil, err
		}
		item, err := identityManagementItem(db, tenant, identity)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]any{"found": false}, nil
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"found": true, "item": item}, nil
	}
	if in.Operation == "list_by_customer" {
		var membership model.TenantMembership
		if err := db.Where("tenant_uuid = ? AND customer_uuid = ?", tenant, in.CustomerUUID).First(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrCustomerAccountNotFound
			}
			return nil, err
		}
		page, size := in.Page, in.PageSize
		if page == 0 {
			page = 1
		}
		if size == 0 {
			size = 20
		}
		query := db.Model(&model.AuthIdentity{}).Where("customer_uuid = ? AND provider = ?", in.CustomerUUID, provider)
		var total int64
		if err := query.Count(&total).Error; err != nil {
			return nil, err
		}
		var rows []model.AuthIdentity
		if err := query.Order("created_at ASC, uuid ASC").Limit(size).Offset((page - 1) * size).Find(&rows).Error; err != nil {
			return nil, err
		}
		items := make([]ExternalIdentityItem, 0, len(rows))
		for _, row := range rows {
			item, err := identityManagementItem(db, tenant, row)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return map[string]any{"items": items, "total": total, "page": page, "page_size": size}, nil
	}
	var item ExternalIdentityItem
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := repo.LockExternalIdentity(tx, provider, in.ProviderSubject); err != nil {
			return err
		}
		var identity model.AuthIdentity
		// Include deleted identities: an old ownership is never silently transferred.
		err := tx.Unscoped().Where("provider = ? AND provider_subject = ?", provider, in.ProviderSubject).First(&identity).Error
		if err == nil {
			if identity.DeletedAt.Valid || identity.Status != model.StatusActive {
				return ErrExternalIdentityConflict
			}
			if in.Operation == "bind" && identity.CustomerUUID != in.CustomerUUID {
				return ErrExternalIdentityConflict
			}
			item, err = identityManagementItem(tx, tenant, identity)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrExternalIdentityConflict
			}
			if err != nil {
				return err
			}
			if in.Operation == "create_and_bind" {
				if item.PrimaryContactUUID == "" || (item.Type != "person" && item.Type != "company") {
					return repo.ErrExternalIdentityContactRequired
				}
				var count int64
				if err := tx.Model(&model.Contact{}).Where("uuid = ? AND tenant_uuid = ? AND customer_uuid = ? AND status = ?", item.PrimaryContactUUID, tenant, item.CustomerUUID, model.ContactStatusActive).Count(&count).Error; err != nil {
					return err
				}
				if count != 1 {
					return repo.ErrExternalIdentityContactRequired
				}
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		customerID := in.CustomerUUID
		if in.Operation == "create_and_bind" {
			profile := in.Customer
			var primary *PrimaryContactInput
			if profile.PrimaryContact != nil {
				p := profile.PrimaryContact
				primary = &PrimaryContactInput{DisplayName: p.DisplayName, GivenName: p.GivenName, FamilyName: p.FamilyName, Email: p.Email, Phone: p.Phone}
			}
			created, err := NewAccountService(tx).Create(ctx, CreateAccountInput{TenantUUID: tenant, Type: profile.Type, DisplayName: profile.DisplayName, Nickname: profile.Nickname, GivenName: profile.GivenName, FamilyName: profile.FamilyName, PrimaryEmail: profile.PrimaryEmail, PrimaryPhone: profile.PrimaryPhone, PrimaryContact: primary, MemberSource: "plugin_identity"})
			if err != nil {
				if errors.Is(err, ErrContactInvalidArgument) || strings.HasPrefix(err.Error(), "customer.primary_contact_required") {
					return ErrCustomerAccountInvalidArgument
				}
				return err
			}
			customerID = created.UUID
		} else {
			var membership model.TenantMembership
			if err := tx.Where("tenant_uuid = ? AND customer_uuid = ?", tenant, customerID).First(&membership).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrCustomerAccountNotFound
				}
				return err
			}
			var account model.Account
			if err := tx.Where("uuid = ?", customerID).First(&account).Error; err != nil {
				return err
			}
			if membership.Status != model.StatusActive || account.Status != model.StatusActive {
				return ErrExternalIdentityConflict
			}
		}
		metadata, _ := json.Marshal(map[string]string{"binding_kind": "plugin_attested", "provider_plugin_id": plugin, "provider_key": provider})
		identity = model.AuthIdentity{CustomerUUID: customerID, Provider: provider, ProviderSubject: in.ProviderSubject, Status: model.StatusActive, Metadata: datatypes.JSON(metadata)}
		if err := tx.Create(&identity).Error; err != nil {
			return err
		}
		meta, _ := json.Marshal(map[string]string{"plugin_id": plugin, "customer_uuid": customerID, "identity_uuid": identity.UUID.String(), "operation": in.Operation, "actor_subject": reqctx.GetSubject(ctx)})
		if err := tx.Create(&audit.AuditEvent{OccurredAt: time.Now().UTC(), TenantUUID: tenant, Source: "customer.external_identity", Operation: in.Operation, ResourceType: "customer.auth_identity", ResourceID: identity.UUID.String(), Outcome: "SUCCESS", Severity: "INFO", CorrelationID: reqctx.GetTraceID(ctx), Meta: datatypes.JSON(meta)}).Error; err != nil {
			return err
		}
		item, err = identityManagementItem(tx, tenant, identity)
		return err
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"item": item}, nil
}
