package delivery

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchDeliveryAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchDeliveryAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchDeliveryAddressLogic {
	return &PatchDeliveryAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchDeliveryAddressLogic) PatchDeliveryAddress(req *types.PatchDeliveryAddressRequest) (resp *types.PatchDeliveryAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
