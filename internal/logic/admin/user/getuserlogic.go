package user

import (
	"PowerX/internal/model/organization"
	"PowerX/internal/model/permission"
	"PowerX/pkg/slicex"
	"context"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserLogic {
	return &GetUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserLogic) GetUser(req *types.GetUserRequest) (resp *types.GetUserReply, err error) {
	user, err := l.svcCtx.PowerX.Organization.FindOneUserByUuid(l.ctx, req.Uuid)
	if err != nil {
		return nil, err
	}

	roles, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetRolesForUser(user.Account)

	var dep *types.UserDepartment
	if user.Department != nil {
		dep = &types.UserDepartment{
			DepId:   user.Department.Id,
			DepName: user.Department.Name,
		}
	}

	vo := types.User{
		Id:            user.Id,
		Account:       user.Account,
		Name:          user.Name,
		Email:         user.Email,
		MobilePhone:   user.MobilePhone,
		Gender:        user.Gender,
		NickName:      user.NickName,
		Desc:          user.NickName,
		Avatar:        user.Avatar,
		ExternalEmail: user.ExternalEmail,
		Department:    dep,
		Roles:         roles,
		JobTitle:      user.JobTitle,
		IsEnabled:     user.Status == organization.UserStatusEnabled,
		CreatedAt:     user.CreatedAt.Format(time.RFC3339),
	}
	if user.Position != nil {
		codes := slicex.SlicePluck(user.Position.Roles, func(item *permission.AdminRole) string {
			return item.RoleCode
		})
		vo.Position = &types.Position{
			Id:        user.Position.Id,
			Name:      user.Position.Name,
			Desc:      user.Position.Desc,
			Level:     user.Position.Level,
			RoleCodes: codes,
		}
		vo.PositionId = user.Position.Id
	}
	return &types.GetUserReply{
		User: &vo,
	}, nil
}
