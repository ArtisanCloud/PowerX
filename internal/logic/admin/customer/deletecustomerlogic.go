package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteCustomerLogic {
	return &DeleteCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteCustomerLogic) DeleteCustomer(req *types.DeleteCustomerRequest) (resp *types.DeleteCustomerReply, err error) {
	err = l.svcCtx.PowerX.Customer.DeleteCustomer(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &types.DeleteCustomerReply{
		CustomerId: req.Id,
	}, nil
}
