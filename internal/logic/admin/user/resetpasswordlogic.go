package user

import (
	"PowerX/internal/types"
	"context"
	"github.com/pkg/errors"

	"PowerX/internal/model"
	"PowerX/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type ResetPasswordLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewResetPasswordLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ResetPasswordLogic {
	return &ResetPasswordLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ResetPasswordLogic) ResetPassword(req *types.ResetPasswordRequest) (resp *types.ResetPasswordReply, err error) {
	user := organization.User{
		Model: model.Model{
			Id: req.UserId,
		},
		Password: "123456",
	}

	err = user.HashPassword()
	if err != nil {
		panic(errors.Wrap(err, "create user hash password failed"))
	}

	if err := l.svcCtx.PowerX.Organization.UpdateUserById(l.ctx, &user, req.UserId); err != nil {
		return nil, err
	}

	return &types.ResetPasswordReply{
		Status: "ok",
	}, nil
}
