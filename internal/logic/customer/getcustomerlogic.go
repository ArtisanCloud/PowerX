package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCustomerLogic {
	return &GetCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetCustomerLogic) GetCustomer(req *types.GetCustomerRequest) (resp *types.GetCustomerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
