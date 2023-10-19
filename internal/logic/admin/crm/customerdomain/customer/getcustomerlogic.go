package customer

import (
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/securityx"
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
		Customer: TransformCustomerToReply(l.svcCtx, mdlCustomer),
	}, nil
}

func TransformCustomerToReply(svcCtx *svc.ServiceContext, mdlCustomer *customerdomain.Customer) (customerReply *types.Customer) {

	var inviter *types.CustomerInviter
	if mdlCustomer.Inviter != nil {
		inviter = &types.CustomerInviter{
			Name:   mdlCustomer.Inviter.Name,
			Mobile: mdlCustomer.Inviter.Mobile,
			Email:  mdlCustomer.Inviter.Email,
		}
	}

	mobile := mdlCustomer.Mobile
	openIdInMiniProgram := mdlCustomer.OpenIdInMiniProgram
	if svcCtx.Config.PowerXDatabase.SeedCommerceData {
		mobile = securityx.MaskMobile(mobile)
		openIdInMiniProgram = securityx.MaskName(openIdInMiniProgram, 20)
	}

	return &types.Customer{
		Id:          mdlCustomer.Id,
		Name:        mdlCustomer.Name,
		Mobile:      mobile,
		Email:       mdlCustomer.Email,
		UUID:        mdlCustomer.Uuid,
		InviterId:   mdlCustomer.InviterId,
		Source:      mdlCustomer.Source,
		Type:        mdlCustomer.Type,
		IsActivated: mdlCustomer.IsActivated,
		CreatedAt:   mdlCustomer.CreatedAt.String(),
		Inviter:     inviter,
		CustomerExternalId: &types.CustomerExternalId{
			OpenIdInMiniProgram:           openIdInMiniProgram,
			OpenIdInWeChatOfficialAccount: mdlCustomer.OpenIdInWeChatOfficialAccount,
			OpenIdInWeCom:                 mdlCustomer.OpenIdInWeCom,
		},
	}

}
