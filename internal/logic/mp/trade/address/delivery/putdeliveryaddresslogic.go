package delivery

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutDeliveryAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutDeliveryAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutDeliveryAddressLogic {
	return &PutDeliveryAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutDeliveryAddressLogic) PutDeliveryAddress(req *types.PutDeliveryAddressRequest) (resp *types.PutDeliveryAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
