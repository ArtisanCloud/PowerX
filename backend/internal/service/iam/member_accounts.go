package iam

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var memberAccountUsernamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,63}$`)

// MemberContactPatch 区分未传字段与主动清空手机号。
type MemberContactPatch struct {
	Email *string `json:"email" validate:"omitempty,email"`
	Phone *string `json:"phone"`
}

func (s *MemberService) updateMemberContactTx(ctx context.Context, tx *gorm.DB, userID uint64, patch MemberContactPatch) error {
	var user m.User
	if err := tx.WithContext(ctx).First(&user, userID).Error; err != nil {
		return err
	}
	if user.IsRoot && !reqctx.IsRoot(ctx) {
		return dto.NewForbidden("租户管理员不能管理 Root 账号", nil)
	}
	fields := map[string]any{}
	for _, field := range []struct {
		name, old string
		value     *string
	}{{"email", user.Email, patch.Email}, {"phone", user.Phone, patch.Phone}} {
		if field.value == nil {
			continue
		}
		value := strings.TrimSpace(*field.value)
		if field.name == "email" {
			value = strings.ToLower(value)
			if value == "" {
				return dto.NewBadRequest("邮箱不能为空", nil)
			}
		}
		fields[field.name] = value
		if field.old == "" || field.old == value {
			continue
		}
		var previous m.Credential
		err := tx.Where("user_id = ? AND provider = ? AND identifier = ?", userID, "password", field.old).First(&previous).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if value == "" {
			if err := tx.Delete(&previous).Error; err != nil {
				return err
			}
			continue
		}
		var existing m.Credential
		err = tx.Where("provider = ? AND identifier = ?", "password", value).First(&existing).Error
		if err == nil {
			if existing.UserID != userID {
				return dto.NewBadRequest("登录标识已被其他账号使用", nil)
			}
			if err := tx.Delete(&previous).Error; err != nil {
				return err
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Model(&previous).Update("identifier", value).Error; err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return tx.Model(&m.User{}).Where("id = ?", userID).Updates(fields).Error
}

// AuthorizeManagement 使用正式 Core 成员管理权限，避免历史 iam.user 被插件占用。
func (s *MemberService) AuthorizeManagement(ctx context.Context, action string) error {
	return s.authorizeMemberAccountPermission(ctx, "member_accounts", action)
}

func (s *MemberService) authorizeMemberAccountPermission(ctx context.Context, resource, action string) error {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.PluginID != "" || claims.Issuer == "powerx-sts" || reqctx.AuthenticatedAPIKeyHash(ctx) != "" {
		return dto.NewUnauthorized("需要用户登录身份", nil)
	}
	if reqctx.IsRoot(ctx) {
		return nil
	}
	tenant := reqctx.GetTenantUUID(ctx)
	memberID := reqctx.GetMemberID(ctx)
	if tenant == "" || memberID == 0 {
		return dto.NewUnauthorized("缺少租户成员身份", nil)
	}
	sources := []string{"platform_capability"}
	if resource == "role" {
		sources = append(sources, "core") // 既有 Core 角色绑定权限仍有效。
	}
	allowed, err := repo.NewPermissionRepository(s.DB).MemberHasLivePermissionFromSources(ctx, tenant, reqctx.GetMemberUUID(ctx), memberID, "iam", resource, action, sources)
	if err != nil {
		return err
	}
	if !allowed {
		if resource == "role" {
			return dto.NewForbidden("缺少角色分配权限（iam.role:bind）", nil)
		}
		return dto.NewForbidden("缺少底座成员管理权限（corex.iam.member_accounts:"+action+"）", nil)
	}
	return nil
}

// ResolveManagedMember 接受正式 UUID，同时兼容现有部门组件使用的数字 ID。
// 两种标识都必须限定当前租户；Root 账号不能由租户管理员修改。
func (s *MemberService) ResolveManagedMember(ctx context.Context, tenant, reference string, includeDeleted ...bool) (*m.Member, error) {
	query := s.DB.WithContext(ctx).Where("tenant_uuid = ?", tenant)
	if len(includeDeleted) > 0 && includeDeleted[0] {
		query = query.Unscoped()
	}
	if id, err := uuid.Parse(reference); err == nil && id != uuid.Nil {
		query = query.Where("uuid = ?", id.String())
	} else if id, err := strconv.ParseUint(reference, 10, 64); err == nil && id != 0 {
		query = query.Where("id = ?", id)
	} else {
		return nil, dto.NewBadRequest("成员标识无效", nil)
	}
	var member m.Member
	if err := query.First(&member).Error; err != nil {
		return nil, err
	}
	var user m.User
	if err := s.DB.WithContext(ctx).First(&user, member.UserID).Error; err != nil {
		return nil, err
	}
	if user.IsRoot && !reqctx.IsRoot(ctx) {
		return nil, dto.NewForbidden("租户管理员不能管理 Root 账号", nil)
	}
	return &member, nil
}

func (s *MemberService) MemberRoleUUIDs(ctx context.Context, tenant string, memberID uint64) ([]string, error) {
	var ids []string
	err := s.DB.WithContext(ctx).Model(&m.RoleBinding{}).
		Where("tenant_uuid = ? AND subject_type = ? AND subject_id = ?", tenant, m.SubMember, memberID).
		Order("role_uuid").Distinct("role_uuid").Pluck("role_uuid", &ids).Error
	if ids == nil {
		ids = []string{}
	}
	return ids, err
}

// replaceMemberRolesTx 与创建/编辑共用一个事务，拒绝系统角色和其他租户角色。
func (s *MemberService) replaceMemberRolesTx(ctx context.Context, tx *gorm.DB, tenant string, memberID uint64, roleUUIDs []string, initial bool) error {
	ids := make([]string, 0, len(roleUUIDs))
	seen := map[string]bool{}
	for _, raw := range roleUUIDs {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			return dto.NewBadRequest("角色 UUID 无效", nil)
		}
		if !seen[id.String()] {
			ids = append(ids, id.String())
			seen[id.String()] = true
		}
	}
	var roles []m.Role
	q := tx.WithContext(ctx).Where("scope = ? AND tenant_uuid = ?", "tenant", tenant)
	if len(ids) == 0 {
		q = q.Where("code = ?", "role_user")
	} else {
		q = q.Where("uuid IN ?", ids)
	}
	if err := q.Find(&roles).Error; err != nil {
		return err
	}
	if len(roles) == 0 || (len(ids) > 0 && len(roles) != len(ids)) {
		return dto.NewBadRequest("角色必须属于当前租户", nil)
	}
	for _, role := range roles {
		if rootrole.IsManaged(role) || role.UUID == uuid.Nil {
			return dto.NewForbidden("不能分配系统 Root 角色", nil)
		}
	}
	// 创建普通用户的默认角色属于 user:write；提升角色或修改已有绑定另需 role:bind。
	if !(initial && len(roles) == 1 && roles[0].Code == "role_user") {
		if err := NewMemberService(tx).authorizeMemberAccountPermission(ctx, "role", "bind"); err != nil {
			return err
		}
	}
	var member m.Member
	if err := tx.Where("tenant_uuid = ? AND id = ?", tenant, memberID).First(&member).Error; err != nil {
		return err
	}
	if err := tx.Where("tenant_uuid = ? AND subject_type = ? AND subject_id = ?", tenant, m.SubMember, memberID).Delete(&m.RoleBinding{}).Error; err != nil {
		return err
	}
	for _, role := range roles {
		binding := m.RoleBinding{TenantUUID: tenant, RoleUUID: role.UUID.String(), RoleID: role.ID, SubjectType: m.SubMember, SubjectUUID: member.UUID.String(), SubjectID: memberID, DataScope: m.ScopeTenant}
		if err := tx.Create(&binding).Error; err != nil {
			return err
		}
	}
	return nil
}

// ResetMemberPassword 先验证租户成员归属，再更新该登录账号已有的密码凭证。
func (s *MemberService) ResetMemberPassword(ctx context.Context, tenant, reference, password string) error {
	if len(password) < 6 || len(password) > 64 {
		return dto.NewBadRequest("密码长度应为 6 至 64 个字符", nil)
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		s := NewMemberService(tx)
		member, err := s.ResolveManagedMember(ctx, tenant, reference)
		if err != nil {
			return err
		}
		var user m.User
		if err := tx.First(&user, member.UserID).Error; err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		// 已有 identifier 全部更新，避免旧密码仍可通过另一个登录入口使用。
		if err := tx.Model(&m.Credential{}).Where("user_id = ? AND provider = ?", user.ID, "password").Update("secret_hash", string(hash)).Error; err != nil {
			return err
		}
		identifier := strings.ToLower(strings.TrimSpace(user.Email))
		if identifier == "" {
			identifier = strings.TrimSpace(user.Phone)
		}
		if identifier == "" {
			identifier = member.Username
		}
		var credential m.Credential
		err = tx.Where("provider = ? AND identifier = ?", "password", identifier).First(&credential).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&m.Credential{UserID: user.ID, Provider: "password", Identifier: identifier, SecretHash: string(hash), IsPrimary: true}).Error
		}
		if err != nil {
			return err
		}
		if credential.UserID != user.ID {
			return dto.NewBadRequest("登录标识已被其他账号使用", nil)
		}
		return nil
	})
}
