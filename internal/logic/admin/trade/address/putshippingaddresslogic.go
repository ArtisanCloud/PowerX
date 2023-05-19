package address

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutShippingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutShippingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutShippingAddressLogic {
	return &PutShippingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutShippingAddressLogic) PutShippingAddress(req *types.PutShippingAddressRequest) (resp *types.PutShippingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
