package billing

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutBillingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutBillingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutBillingAddressLogic {
	return &PutBillingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutBillingAddressLogic) PutBillingAddress(req *types.PutBillingAddressRequest) (resp *types.PutBillingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
