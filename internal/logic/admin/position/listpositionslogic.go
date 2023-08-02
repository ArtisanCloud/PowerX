package position

import (
	"PowerX/internal/model/option"
	"PowerX/internal/model/permission"
	"PowerX/pkg/slicex"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPositionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPositionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPositionsLogic {
	return &ListPositionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPositionsLogic) ListPositions(req *types.ListPositionsRequest) (resp *types.ListPositionsReply, err error) {
	list, err := l.svcCtx.PowerX.Organization.FindManyPositions(l.ctx, &option.FindManyPositionsOption{
		LikeName: req.LikeName,
	})
	typeList := make([]types.Position, len(list))
	for i, item := range list {
		roleCodes := slicex.SlicePluck(item.Roles, func(role *permission.AdminRole) string {
			return role.RoleCode
		})
		typeList[i] = types.Position{
			Id:        item.Id,
			Name:      item.Name,
			Desc:      item.Desc,
			Level:     item.Level,
			RoleCodes: roleCodes,
		}
	}
	return &types.ListPositionsReply{
		List: typeList,
	}, nil
}
