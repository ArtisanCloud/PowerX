package shipping

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/trade"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateShippingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateShippingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateShippingAddressLogic {
	return &CreateShippingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateShippingAddressLogic) CreateShippingAddress(req *types.CreateShippingAddressRequest) (resp *types.CreateShippingAddressReply, err error) {
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	shippingAddress := TransformShippingAddressRequestToShippingAddressToMP(req, authCustomer)

	err = l.svcCtx.PowerX.ShippingAddress.CreateShippingAddress(l.ctx, shippingAddress)

	if err != nil {
		return nil, errorx.WithCause(errorx.ErrCreateObject, err.Error())
	}

	return &types.CreateShippingAddressReply{
		ShippingAddress: TransformShippingAddressToShippingAddressReplyToMP(shippingAddress),
	}, nil

}

func TransformShippingAddressRequestToShippingAddressToMP(req *types.CreateShippingAddressRequest, authCustomer *customerdomain2.Customer) *trade.ShippingAddress {

	return &trade.ShippingAddress{
		CustomerId:   authCustomer.Id,
		Name:         req.Name,
		Recipient:    req.Recipient,
		AddressLine:  req.AddressLine,
		AddressLine2: req.AddressLine2,
		Street:       req.Street,
		City:         req.City,
		Province:     req.Province,
		PostalCode:   req.PostalCode,
		Country:      req.Country,
		PhoneNumber:  req.PhoneNumber,
		IsDefault:    req.IsDefault,
	}
}
