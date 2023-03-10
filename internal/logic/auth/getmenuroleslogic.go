package auth

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
	roles := l.svcCtx.UC.Auth.FindManyAuthRoles(l.ctx)

	rolesMapByMenu := make(map[string]setx.Set[string])
	for _, role := range roles {
		for _, name := range role.MenuNames {
			if set, ok := rolesMapByMenu[name]; ok {
				set.Add(role.RoleCode)
			} else {
				rolesMapByMenu[name] = setx.NewHashSet[string](role.RoleCode)
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
