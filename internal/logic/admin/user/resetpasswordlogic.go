package user

import (
	"PowerX/internal/model/organization"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"

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
	userUuid, err := uuid.Parse(req.UserUuid)
	if err != nil {
		return nil, err
	}
	user := organization.User{
		PowerUUIDModel: powermodel.PowerUUIDModel{
			UUID: userUuid,
		},
		Password: "123456",
	}

	err = user.HashPassword()
	if err != nil {
		panic(errors.Wrap(err, "create user hash password failed"))
	}

	if err := l.svcCtx.PowerX.Organization.UpdateUserByUuid(l.ctx, &user, req.UserUuid); err != nil {
		return nil, err
	}

	return &types.ResetPasswordReply{
		Status: "ok",
	}, nil
}
