package userinfo

import (
	"PowerX/pkg/setx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMenuRolesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMenuRolesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMenuRolesLogic {
	return &GetMenuRolesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMenuRolesLogic) GetMenuRoles() (resp *types.GetMenuRolesReply, err error) {
	roles := l.svcCtx.PowerX.AdminAuthorization.FindAllRoles(l.ctx)

	rolesMapByMenu := make(map[string]setx.Set[string])
	for _, role := range roles {
		for _, roleMenuName := range role.MenuNames {
			if set, ok := rolesMapByMenu[roleMenuName.MenuName]; ok {
				set.Add(role.RoleCode)
			} else {
				rolesMapByMenu[roleMenuName.MenuName] = setx.NewHashSet[string](role.RoleCode)
			}
		}
	}

	var menuRoles []types.MenuRoles
	for m, r := range rolesMapByMenu {
		menuRoles = append(menuRoles, types.MenuRoles{
			MenuName:       m,
			AllowRoleCodes: r.Slice(),
		})
	}

	return &types.GetMenuRolesReply{
		MenuRoles: menuRoles,
	}, nil
}
