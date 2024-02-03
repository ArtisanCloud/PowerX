package auth

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx/crm/customerdomain"
	"PowerX/internal/uc/powerx/wechat"
	"context"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
)

type ValidTokenLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewValidTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ValidTokenLogic {
	return &ValidTokenLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ValidTokenLogic) ValidToken(req *types.MPValidTokenRequest) (resp *types.MPValidTokenReply, err error) {
	res := &types.MPValidTokenReply{
		Valid:  true,
		Reason: "",
	}
	tokenString := req.Token

	secret := l.svcCtx.Config.JWT.MPJWTSecret

	var claims types.TokenClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		if errors.Is(err, jwt.ErrTokenMalformed) {
			res.Valid = false
			res.Reason = "token malformed"
		} else if errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenNotValidYet) {
			res.Valid = false
			res.Reason = "token expired"
		} else {
			res.Valid = false
			res.Reason = "违规Token"

		}
		return res, nil
	}

	// 获取小程序授权的openid
	payload, err := customerdomain.GetPayloadFromToken(token.Raw)
	if err != nil {
		res.Valid = false
		res.Reason = "无效客户信息"
		return res, nil
	}
	vOpenId := payload[customerdomain.AuthCustomerOpenIdKey]

	if vOpenId == nil {
		res.Valid = false
		res.Reason = "无效授权客户OpenId"
		return res, nil
	}
	openId := vOpenId.(string)
	if openId == "" {
		res.Valid = false
		res.Reason = "授权客户OpenId为空"
		return res, nil
	}

	// 小程序的客户记录是否存在
	authMPCustomer, err := l.svcCtx.PowerX.WechatMP.FindOneMPCustomer(l.ctx, &wechat.FindMPCustomerOption{
		OpenIds: []string{openId},
	})
	if err != nil {
		res.Valid = false
		res.Reason = "无效微信小程序客户"
		return res, nil
	}

	// 小程序的客户记录是否存在
	if authMPCustomer.Customer == nil {
		res.Valid = false
		res.Reason = "无效客户记录"
		return res, nil
	}

	return res, nil
}
