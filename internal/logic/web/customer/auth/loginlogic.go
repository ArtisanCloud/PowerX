package auth

import (
	"PowerX/internal/types/errorx"
	customerdomain2 "PowerX/internal/uc/powerx/crm/customerdomain"
	"PowerX/pkg/securityx"
	"context"
	"fmt"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.CustomerLoginRequest) (resp *types.CustomerLoginAuthReply, err error) {

	customer, err := l.svcCtx.PowerX.Customer.GetCustomerByMobile(l.ctx, req.Account)
	if err != nil {
		return nil, err
	}

	if req.Password == "" {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "密码为空")
	}

	//hashPassword = securityx.HashPassword()

	if !securityx.CheckPassword(customer.Password, req.Password) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "密码不正确")
	}

	token := l.svcCtx.PowerX.CustomerAuthorization.SignWebToken(customer, l.svcCtx.Config.JWT.WebJWTSecret)

	return &types.CustomerLoginAuthReply{
		OpenId:      customer.OpenIdInWeChatOfficialAccount,
		PhoneNumber: customer.Mobile,
		NickName:    customer.Name,
		Token: types.WebToken{
			TokenType:    token.TokenType,
			ExpiresIn:    fmt.Sprintf("%d", customerdomain2.CustomerTokenExpiredDuration),
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
		},
	}, nil
}
