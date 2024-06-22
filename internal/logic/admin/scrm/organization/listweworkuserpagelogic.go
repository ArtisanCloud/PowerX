package organization

import (
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/uc/powerx/scrm/wechat"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkUserPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkUserPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkUserPageLogic {
	return &ListWeWorkUserPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListWeWorkUserPage
//
//	@Description: 员工列表
//	@receiver user
//	@param opt
//	@return resp
//	@return err
func (user *ListWeWorkUserPageLogic) ListWeWorkUserPage(opt *types.ListWeWorkUserReqeust) (resp *types.ListWeWorkUserReply, err error) {
	data, err := user.svcCtx.PowerX.SCRM.Wechat.FindManyWechatUsersPage(user.ctx, user.OPT(opt))

	return &types.ListWeWorkUserReply{
		List:      user.DTO(data.List),
		PageIndex: data.PageIndex,
		PageSize:  data.PageSize,
		Total:     data.Total,
	}, err
}

// OPT
//
//	@Description:
//	@receiver user
//	@param opt
//	@return *types.PageOption[wechat.FindManyWechatUsersOption]
func (user *ListWeWorkUserPageLogic) OPT(opt *types.ListWeWorkUserReqeust) *types.PageOption[wechat.FindManyWechatUsersOption] {

	option := types.PageOption[wechat.FindManyWechatUsersOption]{
		Option:    wechat.FindManyWechatUsersOption{},
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
	}
	if opt.Id > 0 {
		option.Option.Ids = []int64{opt.Id}
	}
	if opt.Name != `` {
		option.Option.Names = []string{opt.Name}
	}
	if opt.Alias != `` {
		option.Option.Alias = []string{opt.Alias}
	}
	if opt.Email != `` {
		option.Option.Emails = []string{opt.Email}
	}
	if opt.Mobile != `` {
		option.Option.Mobile = []string{opt.Mobile}
	}
	if opt.OpenUserId != `` {
		option.Option.OpenUserId = []string{opt.OpenUserId}
	}
	if opt.WeWorkMainDepartmentId > 0 {
		option.Option.WeWorkMainDepartmentId = []int64{opt.WeWorkMainDepartmentId}
	}
	if opt.Status > 0 {
		option.Option.Status = []int{opt.Status}
	}
	option.DefaultPageIfNotSet()

	return &option

}

// DTO
//
//	@Description:
//	@receiver user
//	@param data
//	@return users
func (user *ListWeWorkUserPageLogic) DTO(data []*organization.WeWorkUser) (users []*types.WechatUser) {

	for _, val := range data {
		users = append(users, user.dto(val))
	}
	return users

}

// dto
//
//	@Description:
//	@receiver user
//	@param val
//	@return *types.WechatUser
func (user *ListWeWorkUserPageLogic) dto(val *organization.WeWorkUser) *types.WechatUser {
	return &types.WechatUser{
		WeWorkUserId:           val.WeWorkUserId,
		Name:                   val.Name,
		Position:               val.Position,
		Mobile:                 val.Mobile,
		Gender:                 val.Gender,
		Email:                  val.Email,
		BizMail:                val.BizMail,
		Avatar:                 val.Avatar,
		ThumbAvatar:            val.ThumbAvatar,
		Telephone:              val.Telephone,
		Alias:                  val.Alias,
		Address:                val.Address,
		OpenUserId:             val.OpenUserId,
		WeWorkMainDepartmentId: val.WeWorkMainDepartmentId,
		Status:                 val.Status,
		QrCode:                 val.QrCode,
	}
}
