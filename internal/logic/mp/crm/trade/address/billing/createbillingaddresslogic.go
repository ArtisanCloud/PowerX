package billing

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateBillingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateBillingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateBillingAddressLogic {
	return &CreateBillingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateBillingAddressLogic) CreateBillingAddress(req *types.CreateBillingAddressRequest) (resp *types.CreateBillingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
