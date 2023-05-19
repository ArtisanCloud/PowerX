package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutCustomerLogic {
	return &PutCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutCustomerLogic) PutCustomer(req *types.PutCustomerRequest) (resp *types.PutCustomerReply, err error) {
	mdlCustomer := TransformCustomerRequestToCustomer(&(req.Customer))

	// 更新产品对象
	err = l.svcCtx.PowerX.Customer.UpdateCustomer(l.ctx, req.CustomerId, mdlCustomer)

	return &types.PutCustomerReply{
		Customer: TransformCustomerToCustomerReply(mdlCustomer),
	}, err
}
