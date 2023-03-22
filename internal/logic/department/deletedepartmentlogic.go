package department

import (
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx"
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
	depPage := l.svcCtx.UC.Department.FindManyDepartments(l.ctx, &powerx.FindManyDepartmentsOption{
		RootId: req.Id,
	})
	var depIds []int64
	for _, department := range depPage.List {
		depIds = append(depIds, department.ID)
	}

	count := l.svcCtx.UC.Employee.CountEmployees(l.ctx, &powerx.FindEmployeeOption{
		DepIds: depIds,
	})
	if count > 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "删除失败, 要删除的部门下还有用户")
	}

	err = l.svcCtx.UC.Department.DeleteDepartmentById(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &types.DeleteDepartmentReply{
		Id: req.Id,
	}, nil
}
