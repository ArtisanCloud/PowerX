package delivery

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateDeliveryAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateDeliveryAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateDeliveryAddressLogic {
	return &CreateDeliveryAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateDeliveryAddressLogic) CreateDeliveryAddress(req *types.CreateDeliveryAddressRequest) (resp *types.CreateDeliveryAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
