package position

import (
	"PowerX/internal/model/organization"
	"PowerX/internal/model/permission"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreatePositionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreatePositionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePositionLogic {
	return &CreatePositionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreatePositionLogic) CreatePosition(req *types.CreatePositionRequest) (resp *types.CreatePositionReply, err error) {
	roles := make([]*permission.AdminRole, 0, len(req.RoleCodes))
	for _, code := range req.RoleCodes {
		roles = append(roles, &permission.AdminRole{
			RoleCode: code,
		})
	}
	position := organization.Position{
		Name:  req.Name,
		Desc:  req.Desc,
		Roles: roles,
		Level: req.Level,
	}
	err = l.svcCtx.PowerX.Organization.CreatePosition(l.ctx, &position)
	if err != nil {
		return nil, err
	}
	return &types.CreatePositionReply{
		Id: position.Id,
	}, nil
}
