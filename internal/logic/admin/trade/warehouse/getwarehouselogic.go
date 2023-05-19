package warehouse

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetWarehouseLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetWarehouseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetWarehouseLogic {
	return &GetWarehouseLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetWarehouseLogic) GetWarehouse(req *types.GetWarehouseRequest) (resp *types.GetWarehouseResponse, err error) {
	// todo: add your logic here and delete this line

	return
}
