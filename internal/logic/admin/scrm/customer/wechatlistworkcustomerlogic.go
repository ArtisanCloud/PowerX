package customer

import (
    "context"
    "github.com/ArtisanCloud/PowerSocialite/v3/src/models"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/response"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatListWorkCustomerLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatListWorkCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatListWorkCustomerLogic {
    return &WechatListWorkCustomerLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatListWorkCustomer
//  @Description:
//  @receiver this
//  @param opt
//  @return resp
//  @return err
//
func (this *WechatListWorkCustomerLogic) WechatListWorkCustomer(opt *types.WeChatListCustomersRequest) (resp *types.WechatListCustomersReply, err error) {

    data, err := this.svcCtx.PowerX.SCRM.Wechat.CustomerListWechatWorkRequest(opt.UserId)

    return &types.WechatListCustomersReply{
        List: this.DTO(data),
    }, err

}

//
// DTO
//  @Description:
//  @receiver this
//  @param data
//  @return resp
//
func (this *WechatListWorkCustomerLogic) DTO(data []*response.ResponseExternalContact) (resp []*types.WechatCustomer) {

    for _, obj := range data {
        resp = append(resp, this.dto(obj))
    }
    return resp

}

//
// dto
//  @Description:
//  @receiver this
//  @param contact
//  @return *types.WechatCustomer
//
func (this *WechatListWorkCustomerLogic) dto(contact *response.ResponseExternalContact) *types.WechatCustomer {
    return &types.WechatCustomer{
        ExternalContact: this.contact(contact.ExternalContact),
        FollowUser:      this.follow(contact.FollowInfo),
        NextCursor:      ``,
    }
}

//
// contact
//  @Description:
//  @receiver this
//  @param data
//  @return types.WechatCustomersWithExternalContactExternalProfile
//
func (this *WechatListWorkCustomerLogic) contact(data *models.ExternalContact) types.WechatCustomersWithExternalContactExternalProfile {
    return types.WechatCustomersWithExternalContactExternalProfile{
        ExternalUserId: data.ExternalUserID,
        Name:           data.Name,
        Position:       data.Position,
        Avatar:         data.Avatar,
        CorpName:       data.CorpName,
        CorpFullName:   data.CorpFullName,
        Type:           data.Type,
        Gender:         data.Gender,
        Unionid:        data.UnionID,
        ExternalProfile: types.ExternalContactExternalProfileWithExternalProfile{
            this.contactExternalProfile(data.ExternalProfile),
        },
    }
}

//
// follow
//  @Description:
//  @receiver this
//  @param follow
//  @return types.WechatCustomersWithFollowUser
//
func (this *WechatListWorkCustomerLogic) follow(follow *models.FollowUser) types.WechatCustomersWithFollowUser {
    return types.WechatCustomersWithFollowUser{
        UserId:         follow.UserID,
        Remark:         follow.Remark,
        Description:    follow.Description,
        Createtime:     follow.CreateTime,
        Tags:           nil,
        WechatChannels: this.followWechatChannels(follow.WechatChannels),
        RemarkCorpName: follow.RemarkCorpName,
        RemarkMobiles:  follow.RemarkMobiles,
        OpenUserId:     follow.OperUserID,
        AddWay:         follow.AddWay,
        State:          follow.State,
    }
}

//
// contactExternalProfile
//  @Description:
//  @receiver this
//  @param profiles
//  @return externalProfile
//
func (this *WechatListWorkCustomerLogic) contactExternalProfile(profiles *models.ExternalProfile) (externalProfile []*types.ExternalContactExternalProfileExternalProfileWithExternalAttr) {

    if profiles != nil {
        for _, obj := range profiles.ExternalAttr {
            externalProfile = append(externalProfile, &types.ExternalContactExternalProfileExternalProfileWithExternalAttr{
                Type: obj.Type,
                Name: obj.Name,
                Text: types.ExternalContactExternalProfileExternalProfileExternalAttrWithText{obj.Text.Value},
                Web:  types.ExternalContactExternalProfileExternalProfileExternalAttrWithWeb{obj.Web.URL, obj.Web.Title},
                Miniprogram: types.ExternalContactExternalProfileExternalProfileExternalAttrWithMiniprogram{
                    obj.MiniProgram.AppID,
                    obj.MiniProgram.PagePath,
                    obj.MiniProgram.Title,
                },
            })
        }
    }

    return externalProfile
}

//
// followWechatChannels
//  @Description:
//  @receiver this
//  @param channel
//  @return data
//
func (this *WechatListWorkCustomerLogic) followWechatChannels(channel *models.WechatChannel) (data types.WechatCustomersFollowUserWithWechatChannels) {
    if channel != nil {
        data.Nickname = channel.NickName
        data.Source = channel.Source
    }
    return data
}
