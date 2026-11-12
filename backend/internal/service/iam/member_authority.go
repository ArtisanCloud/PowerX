package iam

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const MemberAuthorityCapability = "com.corex.iam.organization.service_read"
const MemberAuthorityEndpoint = "core://iam/organization"
const MemberAuthoritySchema = "powerx.iam.member-authority.v1"

type MemberAuthorityQuery struct {
	Schema      string   `json:"schema"`
	MemberUUIDs []string `json:"member_uuids"`
}
type MemberAuthority struct {
	MemberUUID              string     `json:"member_uuid"`
	UserUUID                string     `json:"user_uuid,omitempty"`
	Status                  string     `json:"status"`
	RoleCodes               []string   `json:"role_codes"`
	DepartmentUUIDs         []string   `json:"department_uuids"`
	DirectManagerMemberUUID *string    `json:"direct_manager_member_uuid"`
	ManagerStatus           string     `json:"manager_status"`
	UpdatedAt               *time.Time `json:"updated_at,omitempty"`
}
type AuthorityDepartment struct {
	DepartmentUUID       string  `json:"department_uuid"`
	Name                 string  `json:"name"`
	ParentDepartmentUUID *string `json:"parent_department_uuid"`
	LeaderMemberUUID     *string `json:"leader_member_uuid"`
	LeaderStatus         string  `json:"leader_status"`
}
type MemberAuthorityBatch struct {
	Schema      string                `json:"schema"`
	TenantUUID  string                `json:"tenant_uuid"`
	EvaluatedAt time.Time             `json:"evaluated_at"`
	Members     []MemberAuthority     `json:"members"`
	Departments []AuthorityDepartment `json:"departments"`
}

// ReadMemberAuthority 仅暴露显式成员角色、部门和实际报告线；不是权限决定。
func (s *TenantDirectoryService) ReadMemberAuthority(ctx context.Context, input MemberAuthorityQuery) (*MemberAuthorityBatch, error) {
	if s == nil || s.db == nil {
		return nil, organizationError(503, "IAM_ORGANIZATION_UNAVAILABLE")
	}
	if !cap.ServiceCredential(ctx) {
		return nil, organizationError(401, "IAM_ORGANIZATION_SERVICE_REQUIRED")
	}
	access, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err == nil {
		err = access.Require(MemberAuthorityCapability)
	}
	if err != nil {
		return nil, pluginAuthorizationAccessError(err)
	}
	tenant := reqctx.GetTenantUUID(ctx)
	if !organizationUUID(tenant) || input.Schema != "powerx.iam.member-authority-query.v1" || len(input.MemberUUIDs) < 1 || len(input.MemberUUIDs) > 200 {
		return nil, organizationError(400, "IAM_ORGANIZATION_INVALID")
	}
	seen := map[string]bool{}
	for _, id := range input.MemberUUIDs {
		if !organizationUUID(id) || seen[id] {
			return nil, organizationError(400, "IAM_ORGANIZATION_INVALID")
		}
		seen[id] = true
	}
	if !s.db.Migrator().HasColumn(&im.Member{}, "direct_manager_member_uuid") {
		return nil, organizationError(503, "IAM_ORGANIZATION_SCHEMA_NOT_READY")
	}
	now := time.Now().UTC()
	result := &MemberAuthorityBatch{Schema: MemberAuthoritySchema, TenantUUID: tenant, EvaluatedAt: now, Members: []MemberAuthority{}, Departments: []AuthorityDepartment{}}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var activeTenant int64
		if err := tx.Model(&tm.Tenant{}).Where("uuid = ? AND status = ?", tenant, 1).Count(&activeTenant).Error; err != nil {
			return err
		}
		if activeTenant != 1 {
			return organizationError(403, "IAM_ORGANIZATION_TENANT_UNAVAILABLE")
		}
		members, err := authorityActiveMembers(tx, tenant, input.MemberUUIDs)
		if err != nil {
			return err
		}
		var bindings []im.RoleBinding
		if err := tx.Where("tenant_uuid = ? AND subject_type = ? AND subject_uuid IN ? AND (start_at IS NULL OR start_at <= ?) AND (end_at IS NULL OR end_at > ?)", tenant, im.SubMember, input.MemberUUIDs, now.UnixMilli(), now.UnixMilli()).Limit(4001).Find(&bindings).Error; err != nil {
			return err
		}
		if len(bindings) > 4000 {
			return organizationError(503, "IAM_ORGANIZATION_LIMIT")
		}
		roleIDs := []string{}
		for _, b := range bindings {
			if !organizationUUID(b.RoleUUID) {
				return organizationError(503, "IAM_ORGANIZATION_IDENTITY_INCOMPLETE")
			}
			roleIDs = append(roleIDs, b.RoleUUID)
		}
		var roles []im.Role
		if len(roleIDs) > 0 {
			if err := tx.Where("uuid IN ? AND (tenant_uuid = ? OR (tenant_uuid = '' AND scope = 'system'))", roleIDs, tenant).Find(&roles).Error; err != nil {
				return err
			}
		}
		roleCodes := map[string]string{}
		for _, r := range roles {
			roleCodes[r.UUID.String()] = string(r.Code)
		}
		var links []im.MemberDepartment
		if err := tx.Where("tenant_uuid = ? AND member_uuid IN ?", tenant, input.MemberUUIDs).Limit(4001).Find(&links).Error; err != nil {
			return err
		}
		if len(links) > 4000 {
			return organizationError(503, "IAM_ORGANIZATION_LIMIT")
		}
		var departments []im.Department
		if err := tx.Where("tenant_uuid = ? AND status = ?", tenant, 1).Limit(2001).Find(&departments).Error; err != nil {
			return err
		}
		if len(departments) > 2000 {
			return organizationError(503, "IAM_ORGANIZATION_LIMIT")
		}
		byDept := map[uuid.UUID]im.Department{}
		for _, d := range departments {
			if d.DepartmentUUID == uuid.Nil {
				return organizationError(503, "IAM_ORGANIZATION_IDENTITY_INCOMPLETE")
			}
			byDept[d.DepartmentUUID] = d
		}
		needed := map[uuid.UUID]bool{}
		byMemberDept := map[string][]string{}
		for _, link := range links {
			if link.MemberUUID == uuid.Nil || link.DepartmentUUID == uuid.Nil {
				return organizationError(503, "IAM_ORGANIZATION_IDENTITY_INCOMPLETE")
			}
			if _, active := members[link.MemberUUID.String()]; !active {
				continue
			}
			if _, exists := byDept[link.DepartmentUUID]; !exists {
				continue
			}
			byMemberDept[link.MemberUUID.String()] = append(byMemberDept[link.MemberUUID.String()], link.DepartmentUUID.String())
			id := link.DepartmentUUID
			chain := map[uuid.UUID]bool{}
			for depth := 0; ; depth++ {
				if depth >= 64 || chain[id] {
					return organizationError(503, "IAM_ORGANIZATION_CYCLE")
				}
				chain[id] = true
				d, ok := byDept[id]
				if !ok {
					return organizationError(503, "IAM_ORGANIZATION_HIERARCHY_UNAVAILABLE")
				}
				needed[id] = true
				if d.ParentDepartmentUUID == nil {
					break
				}
				id = *d.ParentDepartmentUUID
			}
		}
		relatedIDs := []string{}
		for _, m := range members {
			if m.DirectManagerMemberUUID != nil {
				relatedIDs = append(relatedIDs, m.DirectManagerMemberUUID.String())
			}
		}
		for id := range needed {
			if leader := byDept[id].LeaderMemberUUID; leader != nil {
				relatedIDs = append(relatedIDs, leader.String())
			}
		}
		related, err := authorityActiveMembers(tx, tenant, relatedIDs)
		if err != nil {
			return err
		}
		for _, id := range input.MemberUUIDs {
			out := MemberAuthority{MemberUUID: id, Status: "not_found", RoleCodes: []string{}, DepartmentUUIDs: []string{}, ManagerStatus: "unavailable"}
			if m, ok := members[id]; ok {
				stamp := m.UpdatedAt
				out.Status = "active"
				out.UserUUID = m.UserUUID
				out.UpdatedAt = &stamp
				out.ManagerStatus = "unset"
				codes := map[string]bool{}
				for _, b := range bindings {
					if b.SubjectUUID == id {
						if code, ok := roleCodes[b.RoleUUID]; ok {
							codes[code] = true
						}
					}
				}
				for code := range codes {
					out.RoleCodes = append(out.RoleCodes, code)
				}
				sort.Strings(out.RoleCodes)
				out.DepartmentUUIDs = append(out.DepartmentUUIDs, byMemberDept[id]...)
				sort.Strings(out.DepartmentUUIDs)
				if m.DirectManagerMemberUUID != nil {
					out.ManagerStatus = "unavailable"
					manager := m.DirectManagerMemberUUID.String()
					if manager != id {
						if _, active := related[manager]; active {
							out.DirectManagerMemberUUID = &manager
							out.ManagerStatus = "active"
						}
					}
				}
			}
			result.Members = append(result.Members, out)
		}
		for id := range needed {
			d := byDept[id]
			out := AuthorityDepartment{DepartmentUUID: id.String(), Name: d.Name, LeaderStatus: "unset"}
			if d.ParentDepartmentUUID != nil {
				parent := d.ParentDepartmentUUID.String()
				out.ParentDepartmentUUID = &parent
			}
			if d.LeaderMemberUUID != nil {
				out.LeaderStatus = "unavailable"
				leader := d.LeaderMemberUUID.String()
				if _, ok := related[leader]; ok {
					out.LeaderMemberUUID = &leader
					out.LeaderStatus = "active"
				}
			}
			result.Departments = append(result.Departments, out)
		}
		sort.Slice(result.Departments, func(i, j int) bool {
			return result.Departments[i].DepartmentUUID < result.Departments[j].DepartmentUUID
		})
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func authorityActiveMembers(db *gorm.DB, tenant string, ids []string) (map[string]im.Member, error) {
	result := map[string]im.Member{}
	if len(ids) == 0 {
		return result, nil
	}
	var rows []struct {
		im.Member      `gorm:"embedded"`
		ActualUserUUID uuid.UUID `gorm:"column:actual_user_uuid"`
	}
	err := db.Table((&im.Member{}).TableName()+" AS m").Select("m.*, u.uuid AS actual_user_uuid").Joins("JOIN "+(&im.User{}).TableName()+" AS u ON u.id = m.user_id").Where("m.tenant_uuid = ? AND m.uuid IN ? AND m.status = 1 AND m.deleted_at IS NULL AND u.status = 1 AND u.deleted_at IS NULL", tenant, ids).Find(&rows).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	for _, row := range rows {
		if row.UUID == uuid.Nil || !organizationUUID(row.UserUUID) || row.UserUUID != row.ActualUserUUID.String() {
			return nil, organizationError(503, "IAM_ORGANIZATION_IDENTITY_INCOMPLETE")
		}
		result[row.UUID.String()] = row.Member
	}
	return result, nil
}
