package iam

import (
	"context"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
)

// MemberHasLivePlatformPermission 使用 UUID 授权、有效角色绑定和实时 allow 权限。
// 管理员写入不能把已删除、deny、deprecated 或到期授权当作有效许可。
func (r *PermissionRepository) MemberHasLivePlatformPermission(ctx context.Context, tenant, memberUUID string, memberID uint64, module, resource, action string) (bool, error) {
	return r.MemberHasLivePermissionFromSources(ctx, tenant, memberUUID, memberID, module, resource, action, []string{"platform_capability"})
}

// MemberHasLivePermissionFromSources 只接受调用服务明确指定的权限所有者，按 UUID
// 读取实时授权；不让插件占用的旧 triple 或陈旧数字关联成为管理端授权。
func (r *PermissionRepository) MemberHasLivePermissionFromSources(ctx context.Context, tenant, memberUUID string, memberID uint64, module, resource, action string, sources []string) (bool, error) {
	member, err := uuid.Parse(memberUUID)
	if err != nil || member == uuid.Nil || memberID == 0 || len(sources) == 0 {
		return false, nil
	}
	roles, err := NewPluginIdentityRepository(r.db).RoleUUIDs(ctx, tenant, member.String(), memberID, time.Now())
	if err != nil || len(roles) == 0 {
		return false, err
	}
	var count int64
	err = r.db.WithContext(ctx).Model(&model.Permission{}).
		Where("module = ? AND resource = ? AND action = ? AND source IN ? AND status = ? AND effect = ?", module, resource, action, sources, model.PermissionStatusActive, "allow").
		Where("permission_uuid IN (SELECT permission_uuid FROM "+(&model.RolePermission{}).TableName()+" WHERE role_uuid IN ?)", roles).
		Count(&count).Error
	return count > 0, err
}
