package auth

import (
	"PowerX/internal/model"
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	customerdomain2 "PowerX/internal/uc/powerx/customerdomain"
	fmt2 "PowerX/pkg/printx"
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
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
		return nil, errors.New(rs.ErrMSG)
	}
	//fmt2.DD(rs)
	//req = &types.MPCustomerAuthRequest{
	//	IV:            "aggABXMAyD1TQa1OS5pjzA==",
	//	EncryptedData: "VMkaPGYIWUdCwO+MxEBoY6jUs9Ib2uJEQPiDGWnEum9eSHiBEYbGpY+sJn6gPh4PrkyhOMaLH0CuwasVVbaKUS1NHmjEd0Z6pf9W7OyAX4Z3bC8UsEm8PX0YvUPnYnRSMpGdouyOUJu1ie9XCIaqU6j39AZqJfs7bB3aksGN3YHk4EryVIeli9HmrIul9gaa433P/SVJA/34dASdjltv0w==",
	//}
	//rs := &response.ResponseCode2Session{
	//	OpenID:     "o1IFX5A8sfi5nbkXwOzNLLLiL0OA",
	//	SessionKey: "+CG6t0FMK1QMLP4IKWNPUw==",
	//}

	fmt2.Dump(rs, req)
	// 解码手机授权信息
	msgData, errEncrypt := l.svcCtx.PowerX.WechatMP.App.Encryptor.DecryptData(req.EncryptedData, rs.SessionKey, req.IV)

	if errEncrypt != nil {
		return nil, errors.New(errEncrypt.ErrMsg)
	}

	//println(string(msgData))
	// 解析手机信息
	mpPhoneInfo := &model.MPPhoneInfo{}
	err = object.JsonDecode(msgData, mpPhoneInfo)
	if err != nil {
		panic(err.Error())
		return
	}

	mpCustomer := &model.WechatMPCustomer{
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
