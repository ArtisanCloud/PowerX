package customer

import (
	"PowerX/internal/model/scrm/customer"
	"PowerX/internal/uc/powerx/scrm/wechat"
	"context"
	"encoding/json"
	"strings"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkCustomerPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkCustomerPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkCustomerPageLogic {
	return &ListWeWorkCustomerPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// ListWeWorkCustomerPage
//  @Description: 客户列表
//  @receiver customer
//  @param opt
//  @return resp
//  @return err
//
func (customer *ListWeWorkCustomerPageLogic) ListWeWorkCustomerPage(opt *types.WeWorkCustomersRequest) (resp *types.WechatListCustomersReply, err error) {

	data, err := customer.svcCtx.PowerX.SCRM.Wechat.FindManyWeWorkCustomerPage(customer.ctx, customer.OPT(opt), opt.Sync)
	return &types.WechatListCustomersReply{
		List:      customer.DTO(data.List),
		PageIndex: data.PageIndex,
		PageSize:  data.PageSize,
		Total:     data.Total,
	}, err

}

//
// OPT
//  @Description:
//  @receiver customer
//  @param opt
//  @return *types.PageOption[wechat.FindManyWechatCustomerOption]
//
func (customer *ListWeWorkCustomerPageLogic) OPT(opt *types.WeWorkCustomersRequest) *types.PageOption[wechat.FindManyWechatCustomerOption] {

	option := types.PageOption[wechat.FindManyWechatCustomerOption]{
		Option:    wechat.FindManyWechatCustomerOption{},
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
	}
	if v := opt.Name; v != `` {
		option.Option.Name = v
	}
	if v := opt.UserId; v != `` {
		option.Option.UserId = v
	}
	option.DefaultPageIfNotSet()

	return &option

}

//
// DTO
//  @Description:
//  @receiver customer
//  @param data
//  @return resp
//
func (customer *ListWeWorkCustomerPageLogic) DTO(data []*customer.WeWorkExternalContacts) (resp []*types.WechatCustomer) {

	if data != nil {
		for _, obj := range data {
			resp = append(resp, customer.dto(obj))
		}
	}
	return resp

}

//
// dto
//  @Description:
//  @receiver customer
//  @param contact
//  @return *types.WechatCustomer
//
func (customer *ListWeWorkCustomerPageLogic) dto(contact *customer.WeWorkExternalContacts) *types.WechatCustomer {

	return &types.WechatCustomer{
		ExternalContact: types.WechatCustomersWithExternalContactExternalProfile{

			ExternalUserId:  contact.ExternalUserId,
			Name:            contact.Name,
			Position:        contact.Position,
			Avatar:          contact.Avatar,
			CorpName:        contact.CorpName,
			CorpFullName:    contact.CorpFullName,
			Type:            int(contact.WXType),
			Gender:          contact.Gender,
			UnionId:         contact.UnionId,
			UserId:          contact.UserId,
			ExternalProfile: customer.externalContactExternalProfileWithExternalProfile(contact.ExternalProfile),
		},
		FollowUser: customer.wechatCustomersWithFollowUser(&contact.WeWorkExternalContactFollow),
		NextCursor: ``,
	}
}

//
// externalContactExternalProfileWithExternalProfile
//  @Description:
//  @receiver customer
//  @param attr
//  @return data
//
func (customer *ListWeWorkCustomerPageLogic) externalContactExternalProfileWithExternalProfile(attr string) (data types.ExternalContactExternalProfileWithExternalProfile) {
	if attr != `` {
		_ = json.Unmarshal([]byte(attr), &data)
	}
	return data
}

//
// wechatCustomersWithFollowUser
//  @Description:
//  @receiver customer
//  @param follow
//  @return *types.WechatCustomersWithFollowUser
//
func (customer *ListWeWorkCustomerPageLogic) wechatCustomersWithFollowUser(follow *customer.WeWorkExternalContactFollow) *types.WechatCustomersWithFollowUser {

	if follow == nil {
		return nil
	}
	var tags []types.WechatCustomersFollowUserWithTags
	if follow.Tags != `` {
		_ = json.Unmarshal([]byte(follow.Tags), &tags)
	}

	return &types.WechatCustomersWithFollowUser{
		UserId:         follow.UserId,
		Remark:         follow.Remark,
		Description:    follow.Description,
		Createtime:     follow.Createtime,
		Tags:           tags,
		TagIds:         strings.Split(follow.TagIds, `,`),
		WechatChannels: customer.WechatCustomersFollowUserWithWechatChannels(follow.WechatChannels),
		RemarkCorpName: follow.RemarkCorpName,
		RemarkMobiles:  []string{follow.RemarkMobiles},
		OpenUserId:     follow.OpenUserId,
		AddWay:         follow.AddWay,
		State:          follow.State,
	}
}

//
// WechatCustomersFollowUserWithWechatChannels
//  @Description:
//  @receiver customer
//  @param channels
//  @return channel
//
func (customer *ListWeWorkCustomerPageLogic) WechatCustomersFollowUserWithWechatChannels(channels string) (channel types.WechatCustomersFollowUserWithWechatChannels) {

	if channels == `` {
		return channel
	}
	_ = json.Unmarshal([]byte(channels), &channel)

	return channel

}
