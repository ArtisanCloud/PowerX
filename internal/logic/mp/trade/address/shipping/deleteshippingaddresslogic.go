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

type DeleteShippingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteShippingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteShippingAddressLogic {
	return &DeleteShippingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteShippingAddressLogic) DeleteShippingAddress(req *types.DeleteShippingAddressRequest) (resp *types.DeleteShippingAddressReply, err error) {
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	mdlShippingAddress, err := l.svcCtx.PowerX.ShippingAddress.GetShippingAddress(l.ctx, req.ShippingAddressId)
	if err != nil {
		return nil, err
	}
	if authCustomer.Id != mdlShippingAddress.CustomerId {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "您无法删除该地址信息")
	}

	if mdlShippingAddress.IsDefault {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "改地址是默认地址，请先换选其他地址为默认地址，再删除改地址")
	}

	// 删除该对象
	err = l.svcCtx.PowerX.ShippingAddress.DeleteShippingAddress(l.ctx, mdlShippingAddress.Id)
	if mdlShippingAddress.IsDefault {
		return nil, errorx.WithCause(errorx.ErrDeleteObject, err.Error())
	}
	return &types.DeleteShippingAddressReply{
		ShippingAddressId: mdlShippingAddress.Id,
	}, err

}
