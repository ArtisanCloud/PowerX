package billing

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteBillingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteBillingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteBillingAddressLogic {
	return &DeleteBillingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteBillingAddressLogic) DeleteBillingAddress(req *types.DeleteBillingAddressRequest) (resp *types.DeleteBillingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
