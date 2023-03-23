package permission

import (
	"context"

	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

type SetUserRolesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSetUserRolesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetUserRolesLogic {
	return &SetUserRolesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetUserRolesLogic) SetUserRoles() error {
	// todo: add your logic here and delete this line

	return nil
}
