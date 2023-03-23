package permission

import (
	"context"

	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

type SetRolePermissionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSetRolePermissionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetRolePermissionsLogic {
	return &SetRolePermissionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetRolePermissionsLogic) SetRolePermissions() error {
	// todo: add your logic here and delete this line

	return nil
}
