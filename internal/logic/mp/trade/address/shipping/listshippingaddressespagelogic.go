package shipping

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/trade"
	"PowerX/internal/uc/powerx/customerdomain"
	tradeUC "PowerX/internal/uc/powerx/trade"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListShippingAddressesPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListShippingAddressesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListShippingAddressesPageLogic {
	return &ListShippingAddressesPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListShippingAddressesPageLogic) ListShippingAddressesPage(req *types.ListShippingAddressesPageRequest) (resp *types.ListShippingAddressesPageReply, err error) {
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	page, err := l.svcCtx.PowerX.ShippingAddress.FindManyShippingAddresses(l.ctx, &tradeUC.FindManyShippingAddressesOption{
		CustomerId: authCustomer.Id,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformShippingAddressesToShippingAddressesReplyToMP(page.List)
	return &types.ListShippingAddressesPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil

}

func TransformShippingAddressesToShippingAddressesReplyToMP(addresses []*trade.ShippingAddress) []*types.ShippingAddress {
	addressesReply := []*types.ShippingAddress{}
	for _, address := range addresses {
		itemReply := TransformShippingAddressToShippingAddressReplyToMP(address)
		addressesReply = append(addressesReply, itemReply)
	}
	return addressesReply
}

func TransformShippingAddressToShippingAddressReplyToMP(address *trade.ShippingAddress) *types.ShippingAddress {
	return &types.ShippingAddress{
		Id:           address.Id,
		CustomerId:   address.CustomerId,
		Recipient:    address.Recipient,
		AddressLine:  address.AddressLine,
		AddressLine2: address.AddressLine2,
		Street:       address.Street,
		City:         address.City,
		Province:     address.Province,
		PostalCode:   address.PostalCode,
		Country:      address.Country,
		PhoneNumber:  address.PhoneNumber,
		IsDefault:    address.IsDefault,
	}
}
