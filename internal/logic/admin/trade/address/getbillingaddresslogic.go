package address

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetBillingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetBillingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetBillingAddressLogic {
	return &GetBillingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetBillingAddressLogic) GetBillingAddress(req *types.GetBillingAddressRequest) (resp *types.GetBillingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
