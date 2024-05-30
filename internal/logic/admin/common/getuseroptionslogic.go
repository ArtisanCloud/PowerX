package common

import (
	"PowerX/internal/model/option"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserOptionsLogic {
	return &GetUserOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserOptionsLogic) GetUserOptions(req *types.GetUserOptionsRequest) (resp *types.GetUserOptionsReply, err error) {
	userPage := l.svcCtx.PowerX.Organization.FindManyUsersPage(l.ctx, &option.FindManyUsersOption{
		LikeName:        req.LikeName,
		LikeEmail:       req.LikeEmail,
		LikePhoneNumber: req.LikePhoneNumber,
	})

	resp = &types.GetUserOptionsReply{
		PageIndex: userPage.PageIndex,
		PageSize:  userPage.PageSize,
		Total:     userPage.Total,
	}

	var list []types.UserOption
	for _, user := range userPage.List {
		list = append(list, types.UserOption{
			Id:          user.Id,
			Avatar:      user.Avatar,
			Account:     user.Account,
			Name:        user.Name,
			Email:       user.Email,
			PhoneNumber: user.MobilePhone,
		})
	}
	resp.List = list

	return
}
