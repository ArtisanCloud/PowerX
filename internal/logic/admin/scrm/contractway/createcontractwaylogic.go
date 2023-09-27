package contractway

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateContractWayLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateContractWayLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateContractWayLogic {
	return &CreateContractWayLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateContractWayLogic) CreateContractWay(req *types.CreateContractWayRequest) (resp *types.CreateContractWayReply, err error) {
	// todo: add your logic here and delete this line

	return
}
