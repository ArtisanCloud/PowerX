package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutRoleLogic {
	return &PutRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutRoleLogic) PutRole(req *types.PutRoleReqeust) (resp *types.PutRoleReply, err error) {
	// todo: add your logic here and delete this line

	return
}
