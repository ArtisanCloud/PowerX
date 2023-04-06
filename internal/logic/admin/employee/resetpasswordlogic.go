package employee

import (
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx"
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
	employee := powerx.Employee{
		Model: model.Model{
			ID: req.UserId,
		},
		Password: "123456",
	}

	err = employee.HashPassword()
	if err != nil {
		panic(errors.Wrap(err, "create employee hash password failed"))
	}

	if err := l.svcCtx.PowerX.Organization.UpdateEmployeeById(l.ctx, &employee, req.UserId); err != nil {
		return nil, err
	}

	return &types.ResetPasswordReply{
		Status: "ok",
	}, nil
}
