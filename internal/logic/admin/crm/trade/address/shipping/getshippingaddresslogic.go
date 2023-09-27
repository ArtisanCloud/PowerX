package shipping

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetShippingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetShippingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetShippingAddressLogic {
	return &GetShippingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetShippingAddressLogic) GetShippingAddress(req *types.GetShippingAddressRequest) (resp *types.GetShippingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
