package contractway

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateContractWayLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateContractWayLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateContractWayLogic {
	return &UpdateContractWayLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateContractWayLogic) UpdateContractWay(req *types.UpdateContractWayRequest) (resp *types.UpdateContractWayReply, err error) {
	// todo: add your logic here and delete this line

	return
}
