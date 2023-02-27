package permission

import (
	"PowerX/internal/uc"
	"PowerX/pkg/slicex"
	"context"
	"github.com/pkg/errors"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutRoleLogic {
	return &PutRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutRoleLogic) PutRole(req *types.PutRoleReqeust) (resp *types.PutRoleReply, err error) {
	role, err := l.svcCtx.UC.Auth.FindOneAuthRoleByCode(l.ctx, req.RoleCode)
	if err != nil {
		return nil, err
	}
	patch := uc.AuthRole{
		Name:      req.Name,
		Desc:      req.Desc,
		MenuNames: req.MenuNames,
	}

	acts := l.svcCtx.UC.Auth.FindManyRestActsByIds(l.ctx, req.ActIds)
	_, _ = l.svcCtx.UC.Auth.Casbin.RemoveFilteredPolicy(0, role.RoleCode)

	var policies [][]string
	for _, act := range acts {
		policies = append(policies, []string{role.RoleCode, act.FullRestPath, act.Action})
	}

	_, err = l.svcCtx.UC.Auth.Casbin.AddPolicies(policies)
	if err != nil {
		panic(errors.Wrap(err, "casbin add policies failed"))
	}

	l.svcCtx.UC.Auth.PatchRoleByRoleCode(l.ctx, role.RoleCode, &patch)

	actIds := slicex.SlicePluck(acts, func(item *uc.AuthRestAction) int64 {
		return item.ID
	})

	return &types.PutRoleReply{
		AuthRole: &types.AuthRole{
			RoleCode:   patch.RoleCode,
			Name:       patch.Name,
			Desc:       patch.Desc,
			IsReserved: patch.IsReserved,
			ActIds:     actIds,
			MenuNames:  patch.MenuNames,
		},
	}, nil
}
