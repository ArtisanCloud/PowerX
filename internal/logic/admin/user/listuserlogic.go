package user

import (
	"PowerX/internal/model/option"
	"PowerX/internal/model/origanzation"
	"PowerX/internal/model/permission"
	"PowerX/pkg/slicex"
	"context"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListUsersLogic {
	return &ListUsersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListUsersLogic) ListUser(req *types.ListUsersRequest) (resp *types.ListUsersReply, err error) {
	opt := option.FindManyUsersOption{
		Ids:             req.Ids,
		LikeName:        req.LikeName,
		LikeEmail:       req.LikeEmail,
		DepIds:          req.DepIds,
		PositionIDs:     req.PositionIds,
		LikePhoneNumber: req.LikePhoneNumber,
		PageIndex:       req.PageIndex,
		PageSize:        req.PageSize,
	}

	if len(req.RoleCodes) > 0 {
		// bind roles opt, todo improve performance or remove it
		var accounts []string
		for _, code := range req.RoleCodes {
			as, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetUsersForRole(code)
			accounts = append(accounts, as...)
		}
		// 涉及角色查询, root账户会出现在所有角色筛选中
		accounts = append(accounts, "root")
		opt.Accounts = accounts
	}
	if req.IsEnabled != nil {
		if *req.IsEnabled {
			opt.Statuses = append(opt.Statuses, origanzation.UserStatusEnabled)
		} else {
			opt.Statuses = append(opt.Statuses, origanzation.UserStatusDisabled)
		}
	}

	userPage := l.svcCtx.PowerX.Organization.FindManyUsersPage(l.ctx, &opt)

	// build vo
	var vos []types.User
	for _, user := range userPage.List {
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
			Desc:          user.Desc,
			Avatar:        user.Avatar,
			ExternalEmail: user.ExternalEmail,
			Department:    dep,
			Roles:         roles,
			JobTitle:      user.JobTitle,
			IsEnabled:     user.Status == origanzation.UserStatusEnabled,
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

		vos = append(vos, vo)
	}

	return &types.ListUsersReply{
		List:      vos,
		PageIndex: userPage.PageIndex,
		PageSize:  userPage.PageSize,
		Total:     userPage.Total,
	}, nil
}
