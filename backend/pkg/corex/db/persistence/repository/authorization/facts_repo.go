package authorization

import (
	"context"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type Facts struct {
	Roles            []im.Role
	Permissions      []im.Permission
	RolePermissions  []im.RolePermission
	Bindings         []im.RoleBinding
	Assignments      []im.MemberAssignment
	Members          []im.Member
	Users            []im.User
	TenantActive     bool
	PluginActive     bool
	ActiveDimensions map[im.AssignmentDim]map[uint64]bool
}

type Repository struct {
	*base.BaseRepository[im.AuthorizationStream]
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{base.NewBaseRepository[im.AuthorizationStream](db), db}
}

// LoadPermissionFacts 批量读取同一事务中的正式来源，不执行权限合并。
func (r *Repository) LoadPermissionFacts(ctx context.Context, tenant, plugin string) (Facts, error) {
	f := Facts{}
	db := r.db.WithContext(ctx)
	if err := db.Where("tenant_uuid = ? OR scope = ?", tenant, "system").Find(&f.Roles).Error; err != nil {
		return f, err
	}
	if err := db.Where("source = ? AND status = ?", "plugin:"+plugin, im.PermissionStatusActive).Find(&f.Permissions).Error; err != nil {
		return f, err
	}
	roles := []string{}
	for _, x := range f.Roles {
		roles = append(roles, x.UUID.String())
	}
	if len(roles) > 0 {
		if err := db.Where("role_uuid IN ?", roles).Find(&f.RolePermissions).Error; err != nil {
			return f, err
		}
	}
	if err := db.Where("tenant_uuid = ?", tenant).Find(&f.Bindings).Error; err != nil {
		return f, err
	}
	if err := db.Where("tenant_uuid = ?", tenant).Find(&f.Assignments).Error; err != nil {
		return f, err
	}
	f.ActiveDimensions = map[im.AssignmentDim]map[uint64]bool{}
	for _, dim := range []im.AssignmentDim{im.DimOrg, im.DimTeam, im.DimPosition, im.DimGroup} {
		ids := []uint64{}
		for _, a := range f.Assignments {
			if a.DimType == dim {
				ids = append(ids, a.DimID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		var source any
		switch dim {
		case im.DimOrg:
			source = &im.Department{}
		case im.DimTeam:
			source = &im.Team{}
		case im.DimPosition:
			source = &im.Position{}
		case im.DimGroup:
			source = &im.Group{}
		}
		var active []uint64
		if err := db.Model(source).Where("tenant_uuid=? AND id IN ? AND status=?", tenant, ids, 1).Pluck("id", &active).Error; err != nil {
			return f, err
		}
		f.ActiveDimensions[dim] = map[uint64]bool{}
		for _, id := range active {
			f.ActiveDimensions[dim][id] = true
		}
	}
	return f, nil
}

func (r *Repository) Load(ctx context.Context, tenant, plugin string) (Facts, error) {
	f, err := r.LoadPermissionFacts(ctx, tenant, plugin)
	if err != nil {
		return f, err
	}
	db := r.db.WithContext(ctx)
	if err = db.Where("tenant_uuid = ?", tenant).Find(&f.Members).Error; err != nil {
		return f, err
	}
	users := []string{}
	for _, m := range f.Members {
		users = append(users, m.UserUUID)
	}
	if len(users) > 0 {
		if err = db.Where("uuid IN ?", users).Find(&f.Users).Error; err != nil {
			return f, err
		}
	}
	var tenantRow tm.Tenant
	err = db.Where("uuid = ?", tenant).Take(&tenantRow).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return f, err
	}
	f.TenantActive = err == nil && tenantRow.Status == tm.TenantStatusActive
	var instance sm.PluginInstanceConfig
	err = db.Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", tenant, plugin, "auth.credentials").Take(&instance).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return f, err
	}
	f.PluginActive = err == nil && instance.Enabled
	return f, nil
}
