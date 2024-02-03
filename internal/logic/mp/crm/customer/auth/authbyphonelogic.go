package auth

import (
	"PowerX/internal/model"
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/crm/operation"
	"PowerX/internal/model/crm/trade"
	"PowerX/internal/model/wechat"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	customerdomain2 "PowerX/internal/uc/powerx/crm/customerdomain"
	fmt2 "PowerX/pkg/printx"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zeromicro/go-zero/core/logx"
)

type AuthByPhoneLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAuthByPhoneLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AuthByPhoneLogic {
	return &AuthByPhoneLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AuthByPhoneLogic) AuthByPhone(req *types.MPCustomerAuthRequest) (resp *types.MPCustomerLoginAuthReply, err error) {
	//fmt.DD(req)

	// 获取session数据
	rs, err := l.svcCtx.PowerX.WechatMP.App.Auth.Session(l.ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if rs.ErrCode != 0 {
		return nil, errors.New(rs.ErrMsg)
	}

	fmt2.Dump(rs, req)
	// 解码手机授权信息
	msgData, errEncrypt := l.svcCtx.PowerX.WechatMP.App.Encryptor.DecryptData(req.EncryptedData, rs.SessionKey, req.IV)

	if errEncrypt != nil {
		return nil, errors.New(errEncrypt.ErrMsg)
	}
	//println(string(msgData))
	// 解析手机信息
	mpPhoneInfo := &wechat.MPPhoneInfo{}
	err = json.Unmarshal(msgData, mpPhoneInfo)
	if err != nil {
		panic(err.Error())
		return
	}

	fmt2.Dump(mpPhoneInfo)

	mpCustomer := &wechat.WechatMPCustomer{
		OpenId:      rs.OpenID,
		SessionKey:  rs.SessionKey,
		UnionId:     rs.UnionID,
		MPPhoneInfo: *mpPhoneInfo,
	}
	mpCustomer.UniqueID = mpCustomer.GetComposedUniqueID()

	// upsert 小程序客户记录
	mpCustomer, err = l.svcCtx.PowerX.WechatMP.UpsertMPCustomer(l.ctx, mpCustomer)
	if err != nil {
		return nil, err
	}

	source := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, model.TypeSourceChannel, model.ChannelWechat)
	personType := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, customerdomain.TypeCustomerType, customerdomain.CustomerPersonal)

	// upsert 线索
	lead := &customerdomain.Lead{
		Name:        mpCustomer.NickName,
		Mobile:      mpCustomer.PhoneNumber,
		Source:      source,
		Type:        personType,
		IsActivated: true,
		ExternalId: customerdomain.ExternalId{
			OpenIdInMiniProgram: mpCustomer.OpenId,
		},
	}
	lead, err = l.svcCtx.PowerX.Lead.UpsertLead(l.ctx, lead)
	if err != nil {
		return nil, err
	}

	// upsert 客户
	customer := &customerdomain.Customer{
		Name:        mpCustomer.NickName,
		Mobile:      mpCustomer.PhoneNumber,
		Source:      source,
		Type:        personType,
		IsActivated: true,
		ExternalId: customerdomain.ExternalId{
			OpenIdInMiniProgram: mpCustomer.OpenId,
		},
	}
	customer, err = l.svcCtx.PowerX.Customer.UpsertCustomer(l.ctx, customer)
	if err != nil {
		return nil, err
	}

	token := l.svcCtx.PowerX.CustomerAuthorization.SignMPToken(mpCustomer, l.svcCtx.Config.JWT.MPJWTSecret)

	// 创建一个默认的客户会籍
	ddBaseMembershipType := l.svcCtx.PowerX.DataDictionary.GetCachedDD(l.ctx, operation.TypeMembershipType, operation.MembershipTypeBase)
	normalMembership, err := l.svcCtx.PowerX.Membership.GetMembershipBy(l.ctx, customer, ddBaseMembershipType.Id)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrRecordNotFound, err.Error())
	}
	if normalMembership == nil {
		normalMembership = &operation.Membership{
			CustomerId: customer.Id,
			Type:       int(ddBaseMembershipType.Id),
		}
		err = l.svcCtx.PowerX.Membership.CreateMembership(l.ctx, normalMembership)
		if err != nil {
			return nil, errorx.WithCause(errorx.ErrCreateObject, "创建默认会籍发生错误")
		}

		// 如果是第一次有会籍，创建一个虚拟代币账号
		err = l.svcCtx.PowerX.Token.CreateTokenBalance(l.ctx, &trade.TokenBalance{
			CustomerId: customer.Id,
			Balance:    1,
		})
		if err != nil {
			return nil, errorx.WithCause(errorx.ErrCreateObject, "创建默认代币账户发生错误")
		}
		// 赠送1元代币
		err = l.svcCtx.PowerX.Token.CreateTokenExchangeRecord(l.ctx, &trade.TokenExchangeRecord{
			CustomerId:  customer.Id,
			TokenAmount: 1,
		})

	}

	return &types.MPCustomerLoginAuthReply{
		OpenId:      mpCustomer.OpenId,
		UnionId:     mpCustomer.UnionId,
		PhoneNumber: mpCustomer.PhoneNumber,
		NickName:    mpCustomer.NickName,
		AvatarURL:   mpCustomer.AvatarURL,
		Gender:      mpCustomer.Gender,
		Token: types.MPToken{
			TokenType:    token.TokenType,
			ExpiresIn:    fmt.Sprintf("%d", customerdomain2.CustomerTokenExpiredDuration),
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
		},
	}, nil
}
