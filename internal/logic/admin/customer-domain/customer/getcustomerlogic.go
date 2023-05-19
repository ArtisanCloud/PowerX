package customer

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/types/errorx"
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

func (l *GetCustomerLogic) GetCustomer(req *types.GetCustomerReqeuest) (resp *types.GetCustomerReply, err error) {
	mdlCustomer, err := l.svcCtx.PowerX.Customer.GetCustomer(l.ctx, req.Id)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetCustomerReply{
		Customer: TransformCustomerToCustomerReply(mdlCustomer),
	}, nil
}

func TransformCustomerToCustomerReply(mdlCustomer *customerdomain.Customer) (customerReply *types.Customer) {

	var inviter *types.CustomerInviter
	if mdlCustomer.Inviter != nil {
		inviter = &types.CustomerInviter{
			Name:   mdlCustomer.Inviter.Name,
			Mobile: mdlCustomer.Inviter.Mobile,
			Email:  mdlCustomer.Inviter.Email,
		}
	}

	return &types.Customer{
		Id:          mdlCustomer.Id,
		Name:        mdlCustomer.Name,
		Mobile:      mdlCustomer.Mobile,
		Email:       mdlCustomer.Email,
		InviterId:   mdlCustomer.InviterId,
		Source:      mdlCustomer.Source,
		Type:        mdlCustomer.Type,
		IsActivated: mdlCustomer.IsActivated,
		CreatedAt:   mdlCustomer.CreatedAt.String(),
		Inviter:     inviter,
		CustomerExternalId: &types.CustomerExternalId{
			OpenIdInMiniProgram:           mdlCustomer.OpenIdInMiniProgram,
			OpenIdInWeChatOfficialAccount: mdlCustomer.OpenIdInWeChatOfficialAccount,
			OpenIdInWeCom:                 mdlCustomer.OpenIdInWeCom,
		},
	}

}
