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
	member, err := uuid.Parse(memberUUID)
	if err != nil || member == uuid.Nil || memberID == 0 {
		return false, nil
	}
	roles, err := NewPluginIdentityRepository(r.db).RoleUUIDs(ctx, tenant, member.String(), memberID, time.Now())
	if err != nil || len(roles) == 0 {
		return false, err
	}
	var count int64
	err = r.db.WithContext(ctx).Model(&model.Permission{}).
		Where("module = ? AND resource = ? AND action = ? AND source = ? AND status = ? AND effect = ?", module, resource, action, "platform_capability", model.PermissionStatusActive, "allow").
		Where("permission_uuid IN (SELECT permission_uuid FROM "+(&model.RolePermission{}).TableName()+" WHERE role_uuid IN ?)", roles).
		Count(&count).Error
	return count > 0, err
}
