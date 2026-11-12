package iam

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	am "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func organizationError(status int, code string) error {
	return dto.NewErrorWithCode(status, code, "iam.organization."+strings.ToLower(code), nil)
}
func organizationUUID(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}

type ReportingManagerInput struct {
	ManagerMemberUUID *string   `json:"manager_member_uuid"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	Reason            string    `json:"reason"`
}

// SetReportingManager 显式维护直属关系，不从部门负责人或名称推测。
func (s *MemberService) SetReportingManager(ctx context.Context, tenant, member string, input ReportingManagerInput) error {
	if err := s.AuthorizeManagement(ctx, "write"); err != nil {
		return err
	}
	if reqctx.GetTenantUUID(ctx) != tenant || !organizationUUID(tenant) || !organizationUUID(member) || input.ExpectedUpdatedAt.IsZero() || strings.TrimSpace(input.Reason) == "" || len([]rune(input.Reason)) > 500 || input.ManagerMemberUUID != nil && !organizationUUID(*input.ManagerMemberUUID) {
		return organizationError(400, "IAM_ORGANIZATION_INVALID")
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 同租户串行维护报告线，避免并发 A->B、B->A 各自绕过循环核验。
		var tenantRow tm.Tenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND status = ?", tenant, 1).Take(&tenantRow).Error; err != nil {
			return organizationError(409, "IAM_ORGANIZATION_UNAVAILABLE")
		}
		var subject im.Member
		if err := tx.Where("tenant_uuid = ? AND uuid = ? AND status = ?", tenant, member, 1).Take(&subject).Error; err != nil {
			return organizationError(404, "IAM_ORGANIZATION_MEMBER_NOT_FOUND")
		}
		if !subject.UpdatedAt.Equal(input.ExpectedUpdatedAt) {
			return organizationError(409, "IAM_ORGANIZATION_CHANGED")
		}
		var manager *uuid.UUID
		if input.ManagerMemberUUID != nil {
			current := uuid.MustParse(*input.ManagerMemberUUID)
			manager = &current
			seen := map[uuid.UUID]bool{subject.UUID: true}
			for depth := 0; ; depth++ {
				if depth >= 64 || seen[current] {
					return organizationError(409, "IAM_ORGANIZATION_CYCLE")
				}
				seen[current] = true
				var row im.Member
				if err := tx.Where("tenant_uuid = ? AND uuid = ? AND status = ?", tenant, current, 1).Take(&row).Error; err != nil {
					return organizationError(409, "IAM_ORGANIZATION_MANAGER_UNAVAILABLE")
				}
				var active int64
				if err := tx.Model(&im.User{}).Where("id = ? AND status = ?", row.UserID, 1).Count(&active).Error; err != nil {
					return err
				}
				if active != 1 {
					return organizationError(409, "IAM_ORGANIZATION_MANAGER_UNAVAILABLE")
				}
				if row.DirectManagerMemberUUID == nil {
					break
				}
				current = *row.DirectManagerMemberUUID
			}
		}
		updated := tx.Model(&im.Member{}).Where("tenant_uuid = ? AND uuid = ? AND updated_at = ?", tenant, member, input.ExpectedUpdatedAt).Updates(map[string]any{"direct_manager_member_uuid": manager})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return organizationError(409, "IAM_ORGANIZATION_CHANGED")
		}
		before, _ := json.Marshal(map[string]any{"manager_member_uuid": subject.DirectManagerMemberUUID})
		after, _ := json.Marshal(map[string]any{"manager_member_uuid": manager})
		claims := reqctx.GetClaims(ctx)
		meta, _ := json.Marshal(map[string]any{"reason": strings.TrimSpace(input.Reason), "actor_member_uuid": claims.MemberUUID, "actor_user_uuid": claims.UserUUID})
		return tx.Create(&am.AuditEvent{OccurredAt: time.Now().UTC(), TenantUUID: tenant, Source: "iam.organization", Operation: "set_reporting_manager", ResourceType: "iam.member", ResourceID: member, Outcome: "success", Severity: "info", ChangesBefore: before, ChangesAfter: after, Meta: meta}).Error
	})
}
