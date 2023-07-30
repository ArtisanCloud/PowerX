package customer

import (
	"PowerX/internal/model/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchCustomerLogic {
	return &PatchCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchCustomerLogic) PatchCustomer(req *types.PatchCustomerRequest) (resp *types.PatchCustomerReply, err error) {

	mdlCustomer := &customerdomain.Customer{
		Name:        req.Name,
		Email:       req.Email,
		InviterId:   req.InviterId,
		Source:      req.Source,
		Type:        req.Type,
		IsActivated: req.IsActivated,
	}

	// 更新客户对象
	err = l.svcCtx.PowerX.Customer.UpdateCustomer(l.ctx, req.CustomerId, mdlCustomer)

	return &types.PatchCustomerReply{
		Customer: TransformCustomerToCustomerReply(l.svcCtx, mdlCustomer),
	}, err

}
