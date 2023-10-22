package customer

import (
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/pkg/securityx"
	"context"
	"github.com/zeromicro/go-zero/core/logx"
)

type CreateCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCustomerLogic {
	return &CreateCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateCustomerLogic) CreateCustomer(req *types.CreateCustomerRequest) (resp *types.CreateCustomerReply, err error) {

	customer := &customerdomain.Customer{
		Name:        req.Name,
		Mobile:      req.Mobile,
		Email:       req.Email,
		Uuid:        securityx.GenerateUUID(),
		InviterId:   req.InviterId,
		Source:      req.Source,
		Type:        req.Type,
		IsActivated: req.IsActivated,
	}

	err = l.svcCtx.PowerX.Customer.CreateCustomer(l.ctx, customer)

	return &types.CreateCustomerReply{
		customer.Id,
	}, err

}

func TransformRequestToCustomer(customerRequest *types.Customer) (mdlCustomer *customerdomain.Customer) {

	mdlCustomer = &customerdomain.Customer{
		Name: customerRequest.Name,
		//Mobile:      customerRequest.Mobile,
		Email:       customerRequest.Email,
		InviterId:   customerRequest.InviterId,
		Source:      customerRequest.Source,
		Type:        customerRequest.Type,
		IsActivated: customerRequest.IsActivated,
		//ExternalId: customerdomain.ExternalId{
		//	OpenIdInMiniProgram:           customerRequest.CustomerExternalId.OpenIdInMiniProgram,
		//	OpenIdInWeChatOfficialAccount: customerRequest.CustomerExternalId.OpenIdInWeChatOfficialAccount,
		//	OpenIdInWeCom:                 customerRequest.CustomerExternalId.OpenIdInWeCom,
		//},
	}
	mdlCustomer.Id = customerRequest.Id

	return mdlCustomer

}
