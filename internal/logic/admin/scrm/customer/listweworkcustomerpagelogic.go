package customer

import (
	"PowerX/internal/model/scrm/customer"
	"PowerX/internal/uc/powerx/scrm/wechat"
	"context"
	"encoding/json"

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

func (this *ListWeWorkCustomerPageLogic) ListWeWorkCustomerPage(opt *types.WeWorkCustomersRequest) (resp *types.WechatListCustomersReply, err error) {

	data, err := this.svcCtx.PowerX.SCRM.Wechat.FindManyWeWorkCustomerPage(this.ctx, this.OPT(opt), opt.Sync)
	return &types.WechatListCustomersReply{
		List:      this.DTO(data.List),
		PageIndex: data.PageIndex,
		PageSize:  data.PageSize,
		Total:     data.Total,
	}, err

}

//
// OPT
//  @Description:
//  @receiver this
//  @param opt
//  @return *types.PageOption[wechat.FindManyWechatCustomerOption]
//
func (this *ListWeWorkCustomerPageLogic) OPT(opt *types.WeWorkCustomersRequest) *types.PageOption[wechat.FindManyWechatCustomerOption] {

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
//  @receiver this
//  @param data
//  @return resp
//
func (this *ListWeWorkCustomerPageLogic) DTO(data []*customer.WeWorkExternalContacts) (resp []*types.WechatCustomer) {

	if data != nil {
		for _, obj := range data {
			resp = append(resp, this.dto(obj))
		}
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
func (this *ListWeWorkCustomerPageLogic) dto(contact *customer.WeWorkExternalContacts) *types.WechatCustomer {

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
			Unionid:         contact.UnionId,
			UserId:          contact.UserId,
			ExternalProfile: this.externalContactExternalProfileWithExternalProfile(contact.ExternalProfile),
		},
		FollowUser: nil,
		NextCursor: ``,
	}
}

//
// externalContactExternalProfileWithExternalProfile
//  @Description:
//  @receiver this
//  @param attr
//  @return data
//
func (this *ListWeWorkCustomerPageLogic) externalContactExternalProfileWithExternalProfile(attr string) (data types.ExternalContactExternalProfileWithExternalProfile) {
	if attr != `` {
		_ = json.Unmarshal([]byte(attr), &data)
	}
	return data
}
