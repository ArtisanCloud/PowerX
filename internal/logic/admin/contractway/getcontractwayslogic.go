package contractway

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetContractWaysLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetContractWaysLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetContractWaysLogic {
	return &GetContractWaysLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetContractWaysLogic) GetContractWays(req *types.GetContractWaysRequest) (resp *types.GetContractWaysReply, err error) {
	// todo: add your logic here and delete this line

	return
}
