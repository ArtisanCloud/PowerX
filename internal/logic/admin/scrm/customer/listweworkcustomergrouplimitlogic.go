package customer

import (
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/request"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/response"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkCustomerGroupLimitLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkCustomerGroupLimitLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkCustomerGroupLimitLogic {
	return &ListWeWorkCustomerGroupLimitLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (this *ListWeWorkCustomerGroupLimitLogic) ListWeWorkCustomerGroupLimit(opt *types.WeWorkCustomerGroupRequest) (resp *types.WeWorkListCustomerGroupReply, err error) {

	newMap, _ := power.StructToHashMap(opt.OwnerFilter)
	option := &request.RequestGroupChatList{
		StatusFilter: opt.StatusFilter,
		OwnerFilter:  newMap,
		Cursor:       opt.Cursor,
		Limit:        1000,
	}
	if option.Limit == 0 {
		option.Limit = opt.Limit
	}
	list, err := this.svcCtx.PowerX.SCRM.Wechat.PullListWeWorkCustomerGroupRequest(option)

	if list != nil {
		resp = this.DTO(list)
	}

	return resp, err
}

//
// DTO
//  @Description:
//  @receiver this
//  @param data
//  @return *types.WechatListCustomerGroupReply
//
func (this *ListWeWorkCustomerGroupLimitLogic) DTO(data []*response.ResponseGroupChatGet) *types.WeWorkListCustomerGroupReply {

	reply := types.WeWorkListCustomerGroupReply{}
	for _, obj := range data {
		reply.List = append(reply.List, this.dto(obj.GroupChat))
	}

	return &reply

}

//
// dto
//  @Description:
//  @receiver this
//  @param chat
//  @return types.WechatCustomerGroup
//
func (this *ListWeWorkCustomerGroupLimitLogic) dto(chat *response.GroupChat) types.WechatCustomerGroup {

	return types.WechatCustomerGroup{
		ChatId:     chat.ChatID,
		Name:       chat.Name,
		Owner:      chat.Owner,
		CreateTime: chat.CreateTime,
		Notice:     chat.Notice,
		MemberList: this.members(chat.MemberList),
		AdminList:  this.admins(chat.AdminList),
	}
}

//
// members
//  @Description:
//  @receiver this
//  @param members
//  @return list
//
func (this *ListWeWorkCustomerGroupLimitLogic) members(members []*response.Member) (list []*types.WechatCustomerGroupMemberList) {

	for _, val := range members {
		list = append(list, &types.WechatCustomerGroupMemberList{
			Userid:        val.UserID,
			Type:          val.Type,
			JoinTime:      val.JoinTime,
			JoinScene:     val.JoinScene,
			Invitor:       this.weWorkCustomerGroupMemberListInvitor(val.Invitor),
			GroupNickname: val.GroupNickname,
			Name:          val.Name,
			Unionid:       val.UnionID,
		})
	}
	return list

}

//
// admins
//  @Description:
//  @receiver this
//  @param admins
//  @return list
//
func (this *ListWeWorkCustomerGroupLimitLogic) admins(admins []*response.Admin) (list []*types.WechatCustomerGroupAdminList) {

	for _, val := range admins {
		list = append(list, &types.WechatCustomerGroupAdminList{
			Userid: val.UserID,
		})
	}
	return list

}

//
// weWorkCustomerGroupMemberListInvitor
//  @Description:
//  @receiver this
//  @param invitor
//  @return info
//
func (this ListWeWorkCustomerGroupLimitLogic) weWorkCustomerGroupMemberListInvitor(invitor *response.Invitor) (info types.WechatCustomerGroupMemberListInvitor) {
	if invitor != nil {
		info.Userid = invitor.UserID
	}
	return info
}
