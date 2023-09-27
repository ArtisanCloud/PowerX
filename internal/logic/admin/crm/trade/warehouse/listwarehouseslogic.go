package warehouse

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWarehousesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWarehousesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWarehousesLogic {
	return &ListWarehousesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListWarehousesLogic) ListWarehouses(req *types.ListWarehousesRequest) (resp *types.ListWarehousesResponse, err error) {
	// todo: add your logic here and delete this line

	return
}
