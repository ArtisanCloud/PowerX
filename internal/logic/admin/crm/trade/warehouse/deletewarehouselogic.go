package warehouse

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteWarehouseLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteWarehouseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteWarehouseLogic {
	return &DeleteWarehouseLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteWarehouseLogic) DeleteWarehouse(req *types.DeleteWarehouseRequest) (resp *types.DeleteWarehouseResponse, err error) {
	// todo: add your logic here and delete this line

	return
}
