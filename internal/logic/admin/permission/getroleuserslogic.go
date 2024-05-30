package permission

import (
	"PowerX/internal/model/option"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRoleUsersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRoleUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRoleUsersLogic {
	return &GetRoleUsersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRoleUsersLogic) GetRoleUsers(req *types.GetRoleUsersReqeust) (resp *types.GetRoleUsersReply, err error) {
	accounts, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetUsersForRole(req.RoleCode)
	if len(accounts) == 0 {
		return &types.GetRoleUsersReply{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
			Total:     0,
			List:      make([]types.RoleUser, 0),
		}, nil
	}

	userPage := l.svcCtx.PowerX.Organization.FindManyUsersPage(l.ctx, &option.FindManyUsersOption{
		Accounts:  accounts,
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
	})

	resp = &types.GetRoleUsersReply{
		PageIndex: userPage.PageIndex,
		PageSize:  userPage.PageSize,
		Total:     userPage.Total,
	}

	var list []types.RoleUser
	for _, user := range userPage.List {
		var dep *types.RoleUserDepartment
		if user.Department != nil {
			dep = &types.RoleUserDepartment{
				Id:   user.Department.Id,
				Name: user.Department.Name,
			}
		}
		list = append(list, types.RoleUser{
			Id:          user.Id,
			Name:        user.Name,
			Nickname:    user.NickName,
			Account:     user.Account,
			PhoneNumber: user.MobilePhone,
			Department:  dep,
			Email:       user.Email,
		})
	}
	resp.List = list

	return resp, nil
}
