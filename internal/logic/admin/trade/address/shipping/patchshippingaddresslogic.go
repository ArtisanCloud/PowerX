package shipping

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchShippingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchShippingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchShippingAddressLogic {
	return &PatchShippingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchShippingAddressLogic) PatchShippingAddress(req *types.PatchShippingAddressRequest) (resp *types.PatchShippingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
