package customer

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/pkg/securityx"
	"context"

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
	mdlCustomer := TransformRequestToCustomer(&(req.Customer))

	cCustomer, err := l.svcCtx.PowerX.Customer.GetCustomer(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	// 如果当前数据库的用户已经有了UUID
	if cCustomer.Uuid == "" {
		mdlCustomer.Uuid = securityx.GenerateUUIDString()
		mdlCustomer.InviteCode = securityx.GenerateInviteCode(mdlCustomer.Uuid)
	}

	// 更新产品对象
	err = l.svcCtx.PowerX.Customer.UpdateCustomer(l.ctx, req.CustomerId, mdlCustomer)

	return &types.PutCustomerReply{
		Customer: TransformCustomerToReply(l.svcCtx, mdlCustomer),
	}, err
}
