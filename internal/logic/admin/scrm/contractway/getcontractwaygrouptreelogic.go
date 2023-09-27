package contractway

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetContractWayGroupTreeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetContractWayGroupTreeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetContractWayGroupTreeLogic {
	return &GetContractWayGroupTreeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetContractWayGroupTreeLogic) GetContractWayGroupTree(req *types.GetContractWayGroupTreeRequest) (resp *types.GetContractWayGroupTreeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
