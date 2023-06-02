package auth

import (
	"PowerX/internal/model"
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/securityx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterCustomerByPhoneLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterCustomerByPhoneLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterCustomerByPhoneLogic {
	return &RegisterCustomerByPhoneLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterCustomerByPhoneLogic) RegisterCustomerByPhone(req *types.CustomerRegisterByPhoneRequest) (resp *types.CustomerRegisterByPhoneReply, err error) {
	// todo: process code verify

	// todo: process phone verify

	// check customer exist or not
	exist := l.svcCtx.PowerX.Customer.CheckRegisterPhoneExist(l.ctx, req.Phone)
	if exist {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "改手机号已经注册过")
	}

	// hash password
	hashedPassword := securityx.HashPassword(req.Password)

	// register customer by phone
	customerSourceId := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, model.TypeSourceChannel, model.ChannelDirect)
	customerTypeId := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, customerdomain.TypeCustomerType, customerdomain.CustomerPersonal)

	// upsert 客户
	customer := &customerdomain.Customer{
		Mobile:      req.Phone,
		Password:    hashedPassword,
		Source:      customerSourceId,
		Type:        customerTypeId,
		IsActivated: true,
	}
	err = l.svcCtx.PowerX.Customer.CreateCustomer(l.ctx, customer)
	if err != nil {
		return nil, err
	}

	return &types.CustomerRegisterByPhoneReply{
		CustomerId: customer.Id,
	}, nil
}
