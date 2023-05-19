package warehouse

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchWarehouseLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchWarehouseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchWarehouseLogic {
	return &PatchWarehouseLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchWarehouseLogic) PatchWarehouse(req *types.PatchWarehouseRequest) (resp *types.PatchWarehouseResponse, err error) {
	// todo: add your logic here and delete this line

	return
}
