package position

import (
	"PowerX/internal/model/permission"
	"PowerX/pkg/slicex"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPositionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetPositionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPositionLogic {
	return &GetPositionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPositionLogic) GetPosition(req *types.GetPositionRequest) (resp *types.GetPositionReply, err error) {
	position, err := l.svcCtx.PowerX.Organization.FindOnePositionByID(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	roleCodes := slicex.SlicePluck(position.Roles, func(item *permission.AdminRole) string {
		return item.RoleCode
	})

	return &types.GetPositionReply{
		Position: &types.Position{
			Id:        position.Id,
			Name:      position.Name,
			Desc:      position.Desc,
			Level:     position.Level,
			RoleCodes: roleCodes,
		},
	}, nil
}
