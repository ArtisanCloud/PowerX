package department

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDepartmentTreeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDepartmentTreeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDepartmentTreeLogic {
	return &GetDepartmentTreeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDepartmentTreeLogic) GetDepartmentTree(req *types.GetDepartmentTreeRequest) (resp *types.GetDepartmentTreeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
