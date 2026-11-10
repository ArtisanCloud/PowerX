package iam

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

// checkPluginAuthorization 查询已同步到 IAM 的插件权限。查询授权与被查询成员
// 的操作授权分别检查，不把服务调用方提升为被查询用户。
func (s *TenantDirectoryService) checkPluginAuthorization(ctx context.Context, tenant string, in AuthorizationCheckInput) (AuthorizationCheckResult, error) {
	var out AuthorizationCheckResult
	if err := s.requireDB(); err != nil {
		return out, DirectoryUpstreamDependencyError(err)
	}
	canonical, err := canonicalDirectoryTenantUUID(tenant)
	if err != nil || canonical != reqctx.GetTenantUUID(ctx) {
		return out, directoryError(403, DirectoryReasonForbidden, nil)
	}
	pluginID, code := strings.TrimSpace(in.PluginID), strings.TrimSpace(in.PermissionCode)
	if pluginID == "" || code == "" || strings.ContainsAny(pluginID, "/*\\:") {
		return out, DirectoryInvalidArgumentError(errors.New("plugin_id and permission_code are required"))
	}
	access, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err != nil {
		return out, pluginAuthorizationAccessError(err)
	}
	if err = access.Require(directoryAuthorizationCheckCapabilityID); err != nil {
		return out, pluginAuthorizationAccessError(err)
	}
	if c := reqctx.GetClaims(ctx); c != nil && c.Issuer == "powerx-sts" && reqctx.STSIssuanceStatus(ctx) != 0 {
		if reqctx.STSIssuanceStatus(ctx) == 503 {
			return out, DirectoryUpstreamDependencyError(nil)
		}
		return out, directoryError(401, DirectoryReasonUnauthorized, nil)
	}
	var tenantRow tenantmodel.Tenant
	if err = s.db.WithContext(ctx).Where("uuid = ?", canonical).First(&tenantRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, directoryError(403, DirectoryReasonForbidden, nil)
		}
		return out, DirectoryUpstreamDependencyError(err)
	}
	if tenantRow.Status != tenantmodel.TenantStatusActive {
		return out, directoryError(403, DirectoryReasonForbidden, nil)
	}
	var rows []model.Permission
	if err = s.db.WithContext(ctx).Where("source = ? AND status = ?", "plugin:"+pluginID, model.PermissionStatusActive).Find(&rows).Error; err != nil {
		return out, DirectoryUpstreamDependencyError(err)
	}
	var permission *model.Permission
	for i := range rows {
		ownCode := rows[i].Module + "." + rows[i].Resource + ":" + rows[i].Action
		if rows[i].Module == "" {
			ownCode = rows[i].Resource + ":" + rows[i].Action
		}
		if ownCode == code && pluginpermission.EffectivePluginPermissionCode(rows[i]) == code {
			if permission != nil {
				return out, DirectoryUpstreamDependencyError(errors.New("ambiguous plugin permission declaration"))
			}
			permission = &rows[i]
		}
	}
	if permission == nil {
		return out, dto.NewErrorWithCode(404, "IAM_PLUGIN_PERMISSION_NOT_REGISTERED", "IAM_PLUGIN_PERMISSION_NOT_REGISTERED", nil)
	}
	// 所属插件可查询自身目录；其他服务需同时持有对应能力的实时 grant。
	if !access.OwnsPlugin(pluginID) {
		var meta struct {
			CapabilityID string `json:"capability_id"`
		}
		if json.Unmarshal(permission.Meta, &meta) != nil || meta.CapabilityID == "" || !access.Granted[meta.CapabilityID] {
			return out, directoryError(403, DirectoryReasonForbidden, nil)
		}
	}
	if (in.Resource != "" && in.Resource != permission.Resource) || (in.Action != "" && in.Action != permission.Action) {
		return out, DirectoryInvalidArgumentError(errors.New("resource/action do not match registered permission"))
	}
	memberUUID, err := parseDirectoryUUID(in.MemberUUID)
	if err != nil {
		return out, DirectoryInvalidArgumentError(err)
	}
	userUUID, err := parseDirectoryUUID(in.UserUUID)
	if err != nil {
		return out, DirectoryInvalidArgumentError(err)
	}
	var member model.Member
	if err = s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", canonical, memberUUID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, DirectoryMemberNotFoundError(err)
		}
		return out, DirectoryUpstreamDependencyError(err)
	}
	if member.UserUUID != userUUID.String() {
		return out, DirectorySubjectMismatchError(nil)
	}
	var user model.User
	if err = s.db.WithContext(ctx).Where("uuid = ?", userUUID).First(&user).Error; err != nil {
		return out, DirectoryUpstreamDependencyError(err)
	}
	allowed := false
	if member.Status == 1 && user.Status == model.UserStatusActive && permission.Effect == "allow" {
		// UUID role bindings include expiry and member assignments; do not use stale JWT perms.
		if user.IsRoot {
			allowed = true
		} else {
			allowed, err = repo.NewPermissionRepository(s.db).MemberHasLivePermissionFromSources(ctx, canonical, member.UUID.String(), member.ID, permission.Module, permission.Resource, permission.Action, []string{"plugin:" + pluginID})
		}
		if err != nil {
			return out, DirectoryUpstreamDependencyError(err)
		}
	}
	reason := ""
	if !allowed {
		reason = AuthorizationReasonPermissionDenied
	}
	return AuthorizationCheckResult{Allowed: allowed, ReasonCode: reason, TenantUUID: canonical, MemberUUID: member.UUID.String(), UserUUID: member.UserUUID, PluginID: pluginID, PermissionCode: code, Resource: permission.Resource, Action: permission.Action, TraceID: reqctx.GetTraceID(ctx)}, nil
}

func pluginAuthorizationAccessError(err error) error {
	var denied *cap.DirectGrantError
	if errors.As(err, &denied) {
		if denied.Status == 503 {
			return DirectoryUpstreamDependencyError(err)
		}
		return directoryError(denied.Status, DirectoryReasonForbidden, nil)
	}
	return DirectoryUpstreamDependencyError(err)
}
