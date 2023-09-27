package warehouse

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateWarehouseLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateWarehouseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateWarehouseLogic {
	return &UpdateWarehouseLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateWarehouseLogic) UpdateWarehouse(req *types.UpdateWarehouseRequest) (resp *types.UpdateWarehouseResponse, err error) {
	// todo: add your logic here and delete this line

	return
}
