package auth

import (
	"PowerX/internal/model/option"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"time"

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

func (l *LoginLogic) Login(req *types.LoginRequest) (resp *types.LoginReply, err error) {
	if err != nil {
		panic(err)
	}
	opt := option.UserLoginOption{
		Account:     req.UserName,
		PhoneNumber: req.PhoneNumber,
		Email:       req.Email,
	}

	user, err := l.svcCtx.PowerX.Organization.FindOneUserByLoginOption(l.ctx, &opt)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "账户或密码错误")
	}

	if !l.svcCtx.PowerX.Organization.VerifyPassword(user.Password, req.Password) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "账户或密码错误")
	}

	roles, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetRolesForUser(user.Account)

	claims := types.TokenClaims{
		UID:     user.Id,
		Account: user.Account,
		Roles:   roles,
		RegisteredClaims: &jwt.RegisteredClaims{
			Issuer:    "powerx",
			Subject:   user.Account,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 72)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(l.svcCtx.Config.JWT.JWTSecret))
	if err != nil {
		return nil, errors.Wrap(err, "sign token failed")
	}

	return &types.LoginReply{
		Token: signedToken,
	}, nil
}
