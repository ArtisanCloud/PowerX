package shipping

import (
	customerdomain2 "PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/crm/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutShippingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutShippingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutShippingAddressLogic {
	return &PutShippingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutShippingAddressLogic) PutShippingAddress(req *types.PutShippingAddressRequest) (resp *types.PutShippingAddressReply, err error) {

	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	mdlShippingAddress, err := l.svcCtx.PowerX.ShippingAddress.GetShippingAddress(l.ctx, req.ShippingAddressId)
	if err != nil {
		return nil, err
	}
	if authCustomer.Id != mdlShippingAddress.CustomerId {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "您无法修改该地址信息")
	}

	mdlShippingAddress.Name = req.Name
	mdlShippingAddress.Recipient = req.Recipient
	mdlShippingAddress.AddressLine = req.AddressLine
	mdlShippingAddress.AddressLine2 = req.AddressLine2
	mdlShippingAddress.Street = req.Street
	mdlShippingAddress.City = req.City
	mdlShippingAddress.Province = req.Province
	mdlShippingAddress.PostalCode = req.PostalCode
	mdlShippingAddress.Country = req.Country
	mdlShippingAddress.PhoneNumber = req.PhoneNumber
	mdlShippingAddress.IsDefault = req.IsDefault

	// 更新购物项对象
	l.svcCtx.PowerX.ShippingAddress.PatchShippingAddress(l.ctx, mdlShippingAddress.Id, mdlShippingAddress)

	return &types.PutShippingAddressReply{
		ShippingAddress: TransformShippingAddressToReplyForMP(mdlShippingAddress),
	}, err
}
