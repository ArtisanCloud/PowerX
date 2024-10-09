package auth

import (
	"PowerX/internal/model"
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/crm/market"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/securityx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterCustomerByPhoneInInviteCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterCustomerByPhoneInInviteCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterCustomerByPhoneInInviteCodeLogic {
	return &RegisterCustomerByPhoneInInviteCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterCustomerByPhoneInInviteCodeLogic) RegisterCustomerByPhoneInInviteCode(req *types.CustomerRegisterByPhoneInInviteCodeRequest) (resp *types.CustomerRegisterByPhoneReply, err error) {
	// 邀请注册机制
	if req.InviteCode == "" {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "公测阶段，需要邀请码")
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

	// 如果邀请码存在，需要绑定邀请人的UUID
	var record *market.InviteRecord
	if req.InviteCode != "" {
		customer, record, err = l.BindCustomerByInviteCode(l.ctx, req.InviteCode, customer)
		if err != nil {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "邀请码无效")
		}
	}

	// 更新邀请记录的受邀者的ID
	if req.InviteCode != "" {
		record.InviteeID = customer.Id
		l.svcCtx.PowerX.MGM.UpdateInviteRecord(l.ctx, record)
	}

	return &types.CustomerRegisterByPhoneReply{
		CustomerId: customer.Id,
	}, nil
}

func (l *RegisterCustomerByPhoneInInviteCodeLogic) BindCustomerByInviteCode(ctx context.Context,
	inviteCode string, customer *customerdomain.Customer,
) (*customerdomain.Customer, *market.InviteRecord, error) {

	// 通过邀请码，获取邀请人
	inviter, err := l.svcCtx.PowerX.Customer.GetCustomerByInviteCode(ctx, inviteCode)
	if err != nil {
		return nil, nil, err
	}

	// 保存邀请记录
	// 使用默认的MGM规则，直系关系
	mgmSceneId := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(ctx, market.TypeMGMScene, market.MGMSceneDirectRecruitment)
	inviteCode = securityx.GenerateInviteCode(inviter.Uuid)
	record, err := l.svcCtx.PowerX.MGM.CreateInviteRecord(ctx, inviter, customer, inviteCode, mgmSceneId)

	// 绑定邀请人和注册人的关系
	customer.InviterId = inviter.Id
	if err != nil {
		return nil, nil, err
	}

	return customer, record, err
}
