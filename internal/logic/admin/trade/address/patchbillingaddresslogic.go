package address

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchBillingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchBillingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchBillingAddressLogic {
	return &PatchBillingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchBillingAddressLogic) PatchBillingAddress(req *types.PatchBillingAddressRequest) (resp *types.PatchBillingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
