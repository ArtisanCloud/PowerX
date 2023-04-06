package department

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchDepartmentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchDepartmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchDepartmentLogic {
	return &PatchDepartmentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchDepartmentLogic) PatchDepartment(req *types.PatchDepartmentRequest) (resp *types.PatchDepartmentReply, err error) {
	resp = &types.PatchDepartmentReply{}

	return
}
