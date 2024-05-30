package position

import (
	"PowerX/internal/model/option"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeletePositionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeletePositionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePositionLogic {
	return &DeletePositionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeletePositionLogic) DeletePosition(req *types.DeletePositionRequest) (resp *types.DeletePositionReply, err error) {
	userPage := l.svcCtx.PowerX.Organization.FindManyUsersPage(l.ctx, &option.FindManyUsersOption{
		PositionIDs: []int64{req.Id},
	})
	if userPage.Total > 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "该职位下存在员工，无法删除")
	}
	err = l.svcCtx.PowerX.Organization.DeletePosition(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.DeletePositionReply{
		Id: req.Id,
	}, nil
}
