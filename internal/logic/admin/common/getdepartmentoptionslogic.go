package common

import (
	option2 "PowerX/internal/model/option"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDepartmentOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDepartmentOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDepartmentOptionsLogic {
	return &GetDepartmentOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDepartmentOptionsLogic) GetDepartmentOptions(req *types.GetDepartmentOptionsRequest) (resp *types.GetDepartmentOptionsReply, err error) {
	option := types.PageOption[option2.FindManyDepartmentsOption]{
		Option: option2.FindManyDepartmentsOption{
			DepIds:   req.Ids,
			LikeName: req.LikeName,
		},
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
	}
	option.DefaultPageIfNotSet()

	departmentPage := l.svcCtx.PowerX.Organization.FindManyDepartmentsPage(l.ctx, &option)

	resp = &types.GetDepartmentOptionsReply{
		PageIndex: departmentPage.PageIndex,
		PageSize:  departmentPage.PageSize,
		Total:     departmentPage.Total,
	}

	var list []types.DepartmentOption
	for _, department := range departmentPage.List {
		list = append(list, types.DepartmentOption{
			Id:   department.Id,
			Name: department.Name,
		})
	}
	resp.List = list

	return
}
