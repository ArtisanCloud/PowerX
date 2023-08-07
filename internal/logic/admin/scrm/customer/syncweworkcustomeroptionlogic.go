package customer

import (
	"context"
	"github.com/ArtisanCloud/PowerSocialite/v3/src/models"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/response"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncWeWorkCustomerOptionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncWeWorkCustomerOptionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncWeWorkCustomerOptionLogic {
	return &SyncWeWorkCustomerOptionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SyncWeWorkCustomerOption
//
//	@Description: 同步客户信息
//	@receiver cMsg
//	@param opt
//	@return resp
//	@return err
func (cMsg *SyncWeWorkCustomerOptionLogic) SyncWeWorkCustomerOption(opt *types.WeWorkCustomersRequest) (resp *types.WechatListCustomersReply, err error) {

	data, err := cMsg.svcCtx.PowerX.SCRM.Wechat.PullListWeWorkCustomerRequest(opt.UserId)

	return &types.WechatListCustomersReply{
		List: cMsg.DTO(data),
	}, err
}

// DTO
//
//	@Description:
//	@receiver cMsg
//	@param data
//	@return resp
func (cMsg *SyncWeWorkCustomerOptionLogic) DTO(data []*response.ResponseExternalContact) (resp []*types.WechatCustomer) {

	for _, obj := range data {
		resp = append(resp, cMsg.dto(obj))
	}
	return resp

}

// dto
//
//	@Description:
//	@receiver cMsg
//	@param contact
//	@return *types.WechatCustomer
func (cMsg *SyncWeWorkCustomerOptionLogic) dto(contact *response.ResponseExternalContact) *types.WechatCustomer {
	return &types.WechatCustomer{
		ExternalContact: cMsg.contact(contact.ExternalContact),
		FollowUser:      cMsg.follow(contact.FollowInfo),
		NextCursor:      ``,
	}
}

// contact
//
//	@Description:
//	@receiver cMsg
//	@param data
//	@return types.WechatCustomersWithExternalContactExternalProfile
func (cMsg *SyncWeWorkCustomerOptionLogic) contact(data *models.ExternalContact) types.WechatCustomersWithExternalContactExternalProfile {
	return types.WechatCustomersWithExternalContactExternalProfile{
		ExternalUserId: data.ExternalUserID,
		Name:           data.Name,
		Position:       data.Position,
		Avatar:         data.Avatar,
		CorpName:       data.CorpName,
		CorpFullName:   data.CorpFullName,
		Type:           data.Type,
		Gender:         data.Gender,
		UnionId:        data.UnionID,
		ExternalProfile: types.ExternalContactExternalProfileWithExternalProfile{
			cMsg.contactExternalProfile(data.ExternalProfile),
		},
	}
}

// follow
//
//	@Description:
//	@receiver cMsg
//	@param follow
//	@return *types.WechatCustomersWithFollowUser
func (cMsg *SyncWeWorkCustomerOptionLogic) follow(follow *models.FollowUser) *types.WechatCustomersWithFollowUser {

	if follow == nil {
		return nil
	}
	return &types.WechatCustomersWithFollowUser{
		UserId:         follow.UserID,
		Remark:         follow.Remark,
		Description:    follow.Description,
		Createtime:     follow.CreateTime,
		Tags:           nil,
		WechatChannels: cMsg.followWechatChannels(follow.WechatChannels),
		RemarkCorpName: follow.RemarkCorpName,
		RemarkMobiles:  follow.RemarkMobiles,
		OpenUserId:     follow.OperUserID,
		AddWay:         follow.AddWay,
		State:          follow.State,
	}

}

// contactExternalProfile
//
//	@Description:
//	@receiver cMsg
//	@param profiles
//	@return externalProfile
func (cMsg *SyncWeWorkCustomerOptionLogic) contactExternalProfile(profiles *models.ExternalProfile) (externalProfile []*types.ExternalContactExternalProfileExternalProfileWithExternalAttr) {

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

// followWechatChannels
//
//	@Description:
//	@receiver cMsg
//	@param channel
//	@return data
func (cMsg *SyncWeWorkCustomerOptionLogic) followWechatChannels(channel *models.WechatChannel) (data types.WechatCustomersFollowUserWithWechatChannels) {
	if channel != nil {
		data.Nickname = channel.NickName
		data.Source = channel.Source
	}
	return data
}
