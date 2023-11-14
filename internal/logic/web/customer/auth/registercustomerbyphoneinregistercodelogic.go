package auth

import (
	"PowerX/internal/model"
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/securityx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterCustomerByPhoneInRegisterCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterCustomerByPhoneInRegisterCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterCustomerByPhoneInRegisterCodeLogic {
	return &RegisterCustomerByPhoneInRegisterCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterCustomerByPhoneInRegisterCodeLogic) RegisterCustomerByPhoneInRegisterCode(req *types.CustomerRegisterByPhoneInRegisterCodeRequest) (resp *types.CustomerRegisterByPhoneReply, err error) {
	// 限量注册机制
	if req.RegisterCode == "" {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "公测阶段，需要注册码")
	}

	record, err := l.svcCtx.PowerX.RegisterCode.GetRegisterCodeByCode(l.ctx, req.RegisterCode)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "注册码未找到")
	}
	if record == nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "注册码无效")
	}

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
	uuid := securityx.GenerateUUID()
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
	err = l.svcCtx.PowerX.Customer.CreateCustomerByRegisterCode(l.ctx, customer, record)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrCreateObject, "创建注册客户失败")
	}

	return &types.CustomerRegisterByPhoneReply{
		CustomerId: customer.Id,
	}, nil
}
