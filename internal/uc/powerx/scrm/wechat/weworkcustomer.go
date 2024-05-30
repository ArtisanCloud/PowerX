package wechat

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/scrm/customer"
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/types"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerSocialite/v3/src/models"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/response"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

// FindManyWeWorkCustomerPage
//
//	@Description:
//	@receiver this
//	@param ctx
//	@param opt
//	@param sync
//	@return *types.Page[*customer.WeWorkExternalContacts]
//	@return error
func (this wechatUseCase) FindManyWeWorkCustomerPage(ctx context.Context, opt *types.PageOption[FindManyWechatCustomerOption], sync int) (*types.Page[*customer.WeWorkExternalContacts], error) {

	if sync > 0 {
		this.pullSyncWeWorkCustomerRequest(opt.Option.UserId)
	}

	var customers []*customer.WeWorkExternalContacts
	var count int64
	query := this.db.WithContext(ctx).Table(new(customer.WeWorkExternalContacts).TableName() + ` AS a`).
		Joins(fmt.Sprintf(`LEFT JOIN %s AS b ON a.external_user_id=b.external_user_id`, new(customer.WeWorkExternalContactFollow).TableName()))

	if opt.PageIndex == 0 {
		opt.PageIndex = 1
	}
	if opt.PageSize == 0 {
		opt.PageSize = powermodel.PageDefaultSize
	}
	query = buildFindManyCustomerQueryNoPage(query, &opt.Option)

	if err := query.Count(&count).Error; err != nil {
		return nil, err
	}
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	err := query.Preload(`WeWorkExternalContactFollow`).Find(&customers).Error

	return &types.Page[*customer.WeWorkExternalContacts]{
		List:      customers,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, err
}

// buildFindManyCustomerQueryNoPage
//
//	@Description:
//	@param query
//	@param opt
//	@return *gorm.DB
func buildFindManyCustomerQueryNoPage(query *gorm.DB, opt *FindManyWechatCustomerOption) *gorm.DB {

	if v := opt.UserId; v != `` {
		query.Where("a.user_id = ?", v)
	}

	if v := opt.Name; v != `` {
		query.Where("a.name like ?", "%"+v+"%")
	}

	if v := opt.TagId; v != `` {
		query.Where(`POSITION(? IN b.tag_ids) > 0`, v)
	}
	return query
}

// PullListWeWorkCustomerRequest
//
//	@Description:
//	@receiver this
//	@param userID
//	@return []*response.ResponseExternalContact
//	@return error
func (this wechatUseCase) PullListWeWorkCustomerRequest(userID ...string) ([]*response.ResponseExternalContact, error) {

	var err error

	// 外部联系人和客户                                     ExternalContact * externalContact.Client
	// 客户群                                             ExternalContactGroupChat * groupChat.Client
	// 外部联系人和客户                                     ExternalContactContactWay * contactWay.Client
	// 规则                                               ExternalContactCustomerStrategy * customerStrategy.Client
	// 联系客户统计                                        ExternalContactStatistics * statistics.Client
	// 欢迎语                                             ExternalContactGroupWelcomeTemplate * groupWelcomeTemplate.Client
	// 学校                                               ExternalContactSchool * school.Client
	// 朋友圈/发表                                         ExternalContactMoment * moment.Client
	// 规则组                                             ExternalContactMomentStrategy * momentStrategy.Client
	// 企业群发                                            ExternalContactMessageTemplate * messageTemplate.Client
	// 企业标签库                                          ExternalContactTag * tag2.Client
	// 人事变动                                            ExternalContactTransfer * transfer.Client

	info, err := this.wework.ExternalContact.BatchGet(this.ctx, userID, ``, 1000)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.pull.wework.customer.list.error`, info.ResponseWork)

	}
	contacts := []customer.WeWorkExternalContacts{}
	follows := []customer.WeWorkExternalContactFollow{}

	for _, val := range info.ExternalContactList {
		contacts = append(contacts, transferExternalContactToModel(val.ExternalContact, val.FollowInfo.UserID))
		follows = append(follows, transferExternalContactFollowToModel(val.FollowInfo, val.ExternalContact.ExternalUserID))
	}
	err = this.db.Clauses(
		clause.OnConflict{Columns: []clause.Column{{Name: `external_user_id`}}, UpdateAll: true}).CreateInBatches(&contacts, 100).Error
	err = this.db.Clauses(
		clause.OnConflict{Columns: []clause.Column{{Name: `external_user_id`}}, UpdateAll: true}).CreateInBatches(&follows, 100).Error
	if err != nil {
		logx.Errorf(`scrm.wework.customer.contract.error. %v`, err)
	}
	if info != nil {
		return info.ExternalContactList, nil
	}

	return nil, err

}

// transferExternalContactToModel
//
//	@Description:
//	@param contact
//	@return *customer.WeWorkExternalContacts
func transferExternalContactToModel(contact *models.ExternalContact, userID string) customer.WeWorkExternalContacts {
	return customer.WeWorkExternalContacts{

		ExternalUserId:  contact.ExternalUserID,
		AppId:           ``,
		CorpId:          ``,
		OpenId:          ``,
		UnionId:         contact.UnionID,
		UserId:          userID,
		Name:            contact.Name,
		Mobile:          ``,
		Position:        contact.Position,
		Avatar:          contact.Avatar,
		CorpName:        contact.CorpName,
		CorpFullName:    contact.CorpFullName,
		ExternalProfile: ``,
		Gender:          contact.Gender,
		WXType:          int8(contact.Type),
		Status:          1,
		Active:          true,
	}
}

// transferExternalContactFollowToModel
//
//	@Description:
//	@param follow
//	@param externalUserID
//	@return customer.WeWorkExternalContactFollow
func transferExternalContactFollowToModel(follow *models.FollowUser, externalUserID string) customer.WeWorkExternalContactFollow {

	tags, _ := json.Marshal(follow.Tags)
	remarkMobiles, _ := json.Marshal(follow.RemarkMobiles)
	return customer.WeWorkExternalContactFollow{
		ExternalUserId: externalUserID,
		UserId:         follow.UserID,
		Remark:         follow.Remark,
		Description:    follow.Description,
		Createtime:     follow.CreateTime,
		Tags:           string(tags),
		TagIds:         strings.Join(follow.TagIDs, `,`),
		WechatChannels: string(remarkMobiles),
		RemarkCorpName: follow.RemarkCorpName,
		RemarkMobiles:  ``,
		OpenUserId:     follow.OperUserID,
		AddWay:         follow.AddWay,
		State:          follow.State,
	}
}

// pullSyncWeWorkCustomer
//
//	@Description: 全量/增量同步客户信息
//	@receiver this
//	@param ids
func (this *wechatUseCase) pullSyncWeWorkCustomerRequest(ids ...string) {

	if len(ids) > 0 && ids[0] == `` {
		workUsers := this.modelWeworkOrganization.user.Query(this.db)
		ids = organization.AdapterUserSliceUserIDs(func(users []*organization.WeWorkUser) (ids []string) {
			for _, user := range users {
				ids = append(ids, user.WeWorkUserId)
			}
			return ids
		})(workUsers)
	}

	_, _ = this.PullListWeWorkCustomerRequest(ids...)

}
