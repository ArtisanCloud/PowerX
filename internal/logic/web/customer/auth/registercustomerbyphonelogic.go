package auth

import (
	"PowerX/internal/model"
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/securityx"
	"context"

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
	uuid := securityx.GenerateUUIDString()
	inviteCode := securityx.GenerateInviteCode(uuid)
	customer := &customerdomain.Customer{
		Mobile:      req.Phone,
		Password:    hashedPassword,
		Source:      customerSourceId,
		Type:        customerTypeId,
		Uuid:        uuid,
		InviteCode:  inviteCode,
		IsActivated: true,
	}

	// 创建新注册用户
	err = l.svcCtx.PowerX.Customer.CreateCustomer(l.ctx, customer)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrCreateObject, "创建注册客户失败")
	}

	return &types.CustomerRegisterByPhoneReply{
		CustomerId: customer.Id,
	}, nil
}
