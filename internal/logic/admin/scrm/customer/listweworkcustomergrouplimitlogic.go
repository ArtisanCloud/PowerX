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

//
// ListWeWorkCustomerGroupLimit
//  @Description: 客户群列表
//  @receiver cGroup
//  @param opt
//  @return resp
//  @return err
//
func (cGroup *ListWeWorkCustomerGroupLimitLogic) ListWeWorkCustomerGroupLimit(opt *types.WeWorkCustomerGroupRequest) (resp *types.WeWorkListCustomerGroupReply, err error) {

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
    list, err := cGroup.svcCtx.PowerX.SCRM.Wechat.PullListWeWorkCustomerGroupRequest(option)

    if list != nil {
        resp = cGroup.DTO(list)
    }

    return resp, err
}

//
// DTO
//  @Description:
//  @receiver cGroup
//  @param data
//  @return *types.WeWorkListCustomerGroupReply
//
func (cGroup *ListWeWorkCustomerGroupLimitLogic) DTO(data []*response.ResponseGroupChatGet) *types.WeWorkListCustomerGroupReply {

    reply := types.WeWorkListCustomerGroupReply{}
    for _, obj := range data {
        if obj != nil {
            reply.List = append(reply.List, cGroup.dto(obj.GroupChat))
        }
    }

    return &reply

}

//
// dto
//  @Description:
//  @receiver cGroup
//  @param chat
//  @return types.WechatCustomerGroup
//
func (cGroup *ListWeWorkCustomerGroupLimitLogic) dto(chat *response.GroupChat) types.WechatCustomerGroup {

    return types.WechatCustomerGroup{
        ChatId:     chat.ChatID,
        Name:       chat.Name,
        Owner:      chat.Owner,
        CreateTime: chat.CreateTime,
        Notice:     chat.Notice,
        MemberList: cGroup.members(chat.MemberList),
        AdminList:  cGroup.admins(chat.AdminList),
    }
}

//
// members
//  @Description:
//  @receiver cGroup
//  @param members
//  @return list
//
func (cGroup *ListWeWorkCustomerGroupLimitLogic) members(members []*response.Member) (list []*types.WechatCustomerGroupMemberList) {

    for _, val := range members {
        list = append(list, &types.WechatCustomerGroupMemberList{
            UserId:        val.UserID,
            Type:          val.Type,
            JoinTime:      val.JoinTime,
            JoinScene:     val.JoinScene,
            Invitor:       cGroup.weWorkCustomerGroupMemberListInvitor(val.Invitor),
            GroupNickname: val.GroupNickname,
            Name:          val.Name,
            UnionId:       val.UnionID,
        })
    }
    return list

}

//
// admins
//  @Description:
//  @receiver cGroup
//  @param admins
//  @return list
//
func (cGroup *ListWeWorkCustomerGroupLimitLogic) admins(admins []*response.Admin) (list []*types.WechatCustomerGroupAdminList) {

    for _, val := range admins {
        list = append(list, &types.WechatCustomerGroupAdminList{
            UserId: val.UserID,
        })
    }
    return list

}

//
// weWorkCustomerGroupMemberListInvitor
//  @Description:
//  @receiver cGroup
//  @param invitor
//  @return info
//
func (cGroup ListWeWorkCustomerGroupLimitLogic) weWorkCustomerGroupMemberListInvitor(invitor *response.Invitor) (info types.WechatCustomerGroupMemberListInvitor) {
    if invitor != nil {
        info.UserId = invitor.UserID
    }
    return info
}
