package permission

import (
	"context"
	"fmt"
	"strings"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRoleLogic {
	return &GetRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRoleLogic) GetRole(req *types.GetRoleRequest) (resp *types.GetRoleReply, err error) {
	role, err := l.svcCtx.UC.Auth.FindOneAuthRoleByCode(l.ctx, req.RoleCode)
	if err != nil {
		return nil, err
	}
	var keys []string
	policies := l.svcCtx.UC.Auth.Casbin.GetFilteredPolicy(0, role.RoleCode)
	for _, policy := range policies {
		if strings.Contains(policy[2], "|") {
			acts := strings.Split(policy[2], "|")
			for _, act := range acts {
				keys = append(keys, fmt.Sprintf("%s_%s", policy[1], strings.Trim(act, "()")))
			}
		} else {
			keys = append(keys, fmt.Sprintf("%s_%s", policy[1], policy[2]))
		}
	}

	restActs := l.svcCtx.UC.Auth.FindManyRestActsByKeys(l.ctx, keys)
	actIds := make([]int64, 0)
	for _, act := range restActs {
		actIds = append(actIds, act.ID)
	}
	return &types.GetRoleReply{
		AuthRole: &types.AuthRole{
			RoleCode:   role.RoleCode,
			Name:       role.Name,
			Desc:       role.Desc,
			IsReserved: role.IsReserved,
			ActIds:     actIds,
			MenuNames:  role.MenuNames,
		},
	}, nil
	return
}
