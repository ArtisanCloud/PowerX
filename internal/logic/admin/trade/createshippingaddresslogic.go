package trade

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateShippingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateShippingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateShippingAddressLogic {
	return &CreateShippingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateShippingAddressLogic) CreateShippingAddress(req *types.CreateShippingAddressRequest) (resp *types.CreateShippingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
