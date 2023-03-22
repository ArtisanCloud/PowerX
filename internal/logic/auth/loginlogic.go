package auth

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"time"

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
	opt := powerx.FindEmployeeOption{}
	if req.Email != "" {
		opt.Emails = []string{req.Email}
	}
	if req.UserName != "" {
		opt.Accounts = []string{req.UserName}
	}
	if req.PhoneNumber != "" {
		opt.PhoneNumbers = []string{req.PhoneNumber}
	}

	employee, err := l.svcCtx.UC.Employee.FindOneEmployee(l.ctx, &opt)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "账户或密码错误")
	}

	if !l.svcCtx.UC.Employee.VerifyPassword(employee.Password, req.Password) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "账户或密码错误")
	}

	roles, _ := l.svcCtx.UC.Auth.Casbin.GetRolesForUser(employee.Account)

	claims := types.TokenClaims{
		UID:     employee.ID,
		Account: employee.Account,
		Roles:   roles,
		RegisteredClaims: &jwt.RegisteredClaims{
			Issuer:    "powerx",
			Subject:   employee.Account,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 72)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(l.svcCtx.Config.JWTSecret))
	if err != nil {
		return nil, errors.Wrap(err, "sign token failed")
	}

	return &types.LoginReply{
		Token: signedToken,
	}, nil
}
