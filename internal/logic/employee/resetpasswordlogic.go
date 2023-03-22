package employee

import (
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"

	"PowerX/internal/svc"
	"PowerX/internal/types"

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
		Model: &types.Model{
			ID: req.UserId,
		},
		Password: "123456",
	}

	err = employee.HashPassword()
	if err != nil {
		panic(errors.Wrap(err, "create employee hash password failed"))
	}

	l.svcCtx.UC.Employee.UpdateEmployeeById(l.ctx, &employee)

	return &types.ResetPasswordReply{
		Status: "ok",
	}, nil
}
