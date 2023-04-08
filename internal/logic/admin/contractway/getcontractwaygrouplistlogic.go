package contractway

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetContractWayGroupListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetContractWayGroupListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetContractWayGroupListLogic {
	return &GetContractWayGroupListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetContractWayGroupListLogic) GetContractWayGroupList(req *types.GetContractWayGroupListRequest) (resp *types.GetContractWayGroupListReply, err error) {
	// todo: add your logic here and delete this line

	return
}
