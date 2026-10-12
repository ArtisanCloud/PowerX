package customer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	contactrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

type SetPrimaryContactInput struct {
	TenantUUID, CustomerUUID, ContactUUID, ExpectedPrimaryContactUUID string
}

type primaryInitializationKey struct{}

type PrimaryContactResult struct {
	CustomerUUID               string `json:"customer_uuid"`
	PrimaryContactUUID         string `json:"primary_contact_uuid"`
	PreviousPrimaryContactUUID string `json:"previous_primary_contact_uuid"`
}

func hasPrimaryRole(roles []string) bool {
	for _, role := range roles {
		if role == modelcustomer.ContactRolePrimary {
			return true
		}
	}
	return false
}

// SetPrimaryContact 对当前租户 membership 的唯一引用及角色标记进行同一事务维护。
func (s *ContactService) SetPrimaryContact(ctx context.Context, in SetPrimaryContactInput) (PrimaryContactResult, error) {
	if s == nil || s.db == nil {
		return PrimaryContactResult{}, gorm.ErrInvalidDB
	}
	tenant, customer, err := canonicalContactScope(in.TenantUUID, in.CustomerUUID)
	if err != nil {
		return PrimaryContactResult{}, err
	}
	contact, err := canonicalUUID(in.ContactUUID)
	if err != nil {
		return PrimaryContactResult{}, err
	}
	expected := strings.TrimSpace(in.ExpectedPrimaryContactUUID)
	if expected != "" {
		if expected, err = canonicalUUID(expected); err != nil {
			return PrimaryContactResult{}, err
		}
	}
	var result PrimaryContactResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureActiveCustomer(ctx, tx, tenant, customer); err != nil {
			return err
		}
		repo := contactrepo.NewContactRepository(tx)
		membership, err := repo.LockMembership(ctx, tenant, customer)
		if err != nil {
			return err
		}
		target, err := getScopedContact(ctx, repo, tenant, customer, contact)
		if err != nil {
			return err
		}
		if target.Status != modelcustomer.ContactStatusActive {
			return dto.NewErrorWithCode(409, "CONTACT_PRIMARY_NOT_ACTIVE", "主要联系人必须处于 active 状态", nil)
		}
		result = PrimaryContactResult{CustomerUUID: customer, PrimaryContactUUID: contact, PreviousPrimaryContactUUID: membership.PrimaryContactUUID}
		if membership.PrimaryContactUUID != expected {
			return dto.NewErrorWithCode(409, "CONTACT_PRIMARY_CHANGED", "主要联系人引用已变化，请刷新后重新确认", nil)
		}
		if err := ensureActiveCustomer(ctx, tx, tenant, customer); err != nil {
			return err
		}
		rows, err := repo.AllForCustomer(ctx, tenant, customer)
		if err != nil {
			return err
		}
		changed := membership.PrimaryContactUUID != contact
		for _, row := range rows {
			var roles []string
			if err := json.Unmarshal(row.Roles, &roles); err != nil {
				return ErrContactInvalidArgument
			}
			kept := []string{}
			for _, role := range roles {
				if role != modelcustomer.ContactRolePrimary {
					kept = append(kept, role)
				}
			}
			if row.UUID.String() == contact {
				kept = append(kept, modelcustomer.ContactRolePrimary)
			}
			rolesJSON := marshalStringArray(kept)
			if string(row.Roles) == string(rolesJSON) {
				continue
			}
			row.Roles = rolesJSON
			changed = true
			if err := repo.Save(ctx, &row); err != nil {
				return err
			}
		}
		if !changed {
			return nil
		}
		if err := repo.SetPrimaryReference(ctx, membership, contact); err != nil {
			return err
		}
		return recordContactAudit(ctx, tx, tenant, customer, contact, "customer.contact.primary_changed", map[string]any{"previous_primary_contact_uuid": result.PreviousPrimaryContactUUID, "primary_contact_uuid": contact})
	})
	if err != nil && dto.CodeOf(err) == "" && !errors.Is(err, ErrContactInvalidArgument) && !errors.Is(err, ErrContactNotFound) && !errors.Is(err, ErrContactCustomerMismatch) && !errors.Is(err, ErrContactCustomerMembershipInactive) {
		return PrimaryContactResult{}, dto.NewErrorWithCode(500, "CONTACT_PRIMARY_OPERATION_FAILED", "主联系人事务未提交，请保存 Trace 并刷新回读", nil)
	}
	return result, err
}
