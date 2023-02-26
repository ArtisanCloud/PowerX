package department

import (
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateDepartmentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateDepartmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateDepartmentLogic {
	return &CreateDepartmentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateDepartmentLogic) CreateDepartment(req *types.CreateDepartmentRequest) (resp *types.CreateDepartmentReply, err error) {
	// 校验leaderIds是否全部存在
	count := l.svcCtx.UC.Employee.CountEmployees(l.ctx, &uc.FindEmployeeOption{
		Ids: req.LeaderIds,
	})
	if count != int64(len(req.LeaderIds)) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "部分部门负责人不存在")
	}

	dep := uc.Department{
		Name:      req.DepName,
		LeaderIds: req.LeaderIds,
		Desc:      req.Desc,
		PId:       req.PId,
	}

	err = l.svcCtx.UC.Department.CreateDepartment(l.ctx, &dep)
	if err != nil {
		return nil, err
	}

	return &types.CreateDepartmentReply{
		Id: dep.ID,
	}, nil
}
