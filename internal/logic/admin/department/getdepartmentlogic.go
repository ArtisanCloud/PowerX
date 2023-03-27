package department

import (
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
	department, err := l.svcCtx.PowerX.Organization.FindOneDepartment(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	resp = &types.GetDepartmentReply{
		Department: &types.Department{
			Id:          department.ID,
			DepName:     department.Name,
			PhoneNumber: department.PhoneNumber,
			Email:       department.Email,
			Remark:      department.Remark,
		},
	}

	if department.Leader != nil {
		resp.Leader = types.DepartmentLeader{
			Id:       department.Leader.ID,
			Name:     department.Leader.Name,
			NickName: department.Leader.NickName,
			Avatar:   department.Leader.Avatar,
		}
	}

	return resp, nil
}
