package department

import (
	"PowerX/internal/uc"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDepartmentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDepartmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDepartmentLogic {
	return &GetDepartmentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDepartmentLogic) GetDepartment(req *types.GetDepartmentRequest) (resp *types.GetDepartmentReply, err error) {
	department, err := l.svcCtx.UC.Department.FindOneDepartment(l.ctx, &uc.FindOneDepartmentOption{
		Id: &req.Id,
	})
	if err != nil {
		return nil, err
	}

	return &types.GetDepartmentReply{
		Department: &types.Department{
			Id:          department.ID,
			DepName:     department.Name,
			LeaderIds:   department.LeaderIds,
			PhoneNumber: department.PhoneNumber,
			Email:       department.Email,
			Remark:      department.Remark,
		},
	}, nil
}
