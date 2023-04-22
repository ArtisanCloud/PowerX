package common

import (
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetEmployeeOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetEmployeeOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetEmployeeOptionsLogic {
	return &GetEmployeeOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetEmployeeOptionsLogic) GetEmployeeOptions(req *types.GetEmployeeOptionsRequest) (resp *types.GetEmployeeOptionsReply, err error) {
	employeePage := l.svcCtx.PowerX.Organization.FindManyEmployeesPage(l.ctx, &powerx.FindManyEmployeesOption{
		LikeName:        req.LikeName,
		LikeEmail:       req.LikeEmail,
		LikePhoneNumber: req.LikePhoneNumber,
	})

	resp = &types.GetEmployeeOptionsReply{
		PageIndex: employeePage.PageIndex,
		PageSize:  employeePage.PageSize,
		Total:     employeePage.Total,
	}

	var list []types.EmployeeOption
	for _, employee := range employeePage.List {
		list = append(list, types.EmployeeOption{
			Id:          employee.Id,
			Avatar:      employee.Avatar,
			Account:     employee.Account,
			Name:        employee.Name,
			Email:       employee.Email,
			PhoneNumber: employee.MobilePhone,
		})
	}
	resp.List = list

	return
}
