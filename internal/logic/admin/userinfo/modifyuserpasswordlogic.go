package userinfo

import (
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ModifyUserPasswordLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewModifyUserPasswordLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ModifyUserPasswordLogic {
	return &ModifyUserPasswordLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ModifyUserPasswordLogic) ModifyUserPassword(req *types.ModifyPasswordReqeust) error {
	cred, err := l.svcCtx.PowerX.AdminAuthorization.AuthMetadataFromContext(l.ctx)
	if err != nil {
		panic(errors.Wrap(err, "get user metadata failed"))
	}

	err = l.svcCtx.PowerX.Organization.PatchEmployeeByUserId(l.ctx, &powerx.Employee{Password: req.Password}, cred.UID)
	if err != nil {
		return err
	}
	return nil
}
