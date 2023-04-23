package department

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteDepartmentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteDepartmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteDepartmentLogic {
	return &DeleteDepartmentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteDepartmentLogic) DeleteDepartment(req *types.DeleteDepartmentRequest) (resp *types.DeleteDepartmentReply, err error) {
	// 检查部门下是否还有员工
	deps, err := l.svcCtx.PowerX.Organization.FindManyDepartmentsByRootId(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	var depIds []int64
	for _, department := range deps {
		depIds = append(depIds, department.Id)
	}

	if l.svcCtx.PowerX.Organization.CountEmployeeInDepartmentByIds(l.ctx, depIds) > 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "删除失败, 要删除的部门下还有用户")
	}

	err = l.svcCtx.PowerX.Organization.DeleteDepartmentById(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &types.DeleteDepartmentReply{
		Id: req.Id,
	}, nil
}
