package wechat

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/scrm/customer"
	"PowerX/internal/model/scrm/tag"
	"PowerX/internal/types"
	baseResp "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/response"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/tag/request"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/tag/response"
	"strings"
	"time"
)

// FindListWeWorkTagPage
//
//	@Description:
//	@receiver this
//	@param option
//	@return reply
//	@return err
func (this wechatUseCase) FindListWeWorkTagGroupOption() (reply []*tag.WeWorkTagGroup, err error) {

	reply = this.modelWeworkTag.group.Query(this.db)

	return reply, err

}

// FindListWeWorkTagGroupPage
//
//	@Description:
//	@receiver this
//	@param option
//	@return reply
//	@return err
func (this wechatUseCase) FindListWeWorkTagGroupPage(option *types.PageOption[types.ListWeWorkTagGroupPageRequest]) (reply *types.Page[*tag.WeWorkTagGroup], err error) {

	var tagGroups []*tag.WeWorkTagGroup
	var count int64
	query := this.db.WithContext(this.ctx).Model(tag.WeWorkTagGroup{}).Where(`is_delete = ?`, false)

	if v := option.Option.GroupId; v != `` {
		query.Where(`group_id = ?`, v)
	}

	if v := option.Option.GroupName; v != `` {
		query.Where(`name like ?`, "%"+v+"%")
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, err
	}
	if option.PageIndex != 0 && option.PageSize != 0 {
		query.Offset((option.PageIndex - 1) * option.PageSize).Limit(option.PageSize)
	}

	err = query.Preload(`WeWorkGroupTags`).Find(&tagGroups).Error

	return &types.Page[*tag.WeWorkTagGroup]{
		List:      tagGroups,
		PageIndex: option.PageIndex,
		PageSize:  option.PageSize,
		Total:     count,
	}, err

}

// FindListWeWorkTagOption
//
//	@Description:
//	@receiver this
//	@return reply
//	@return err
func (this wechatUseCase) FindListWeWorkTagOption() (reply []*tag.WeWorkTag, err error) {

	reply = this.modelWeworkTag.tag.Query(this.db)

	return reply, err

}

// FindListWeWorkTagPage
//
//	@Description:
//	@receiver this
//	@param option
//	@return reply
//	@return err
func (this wechatUseCase) FindListWeWorkTagPage(option *types.PageOption[types.ListWeWorkTagReqeust]) (reply *types.Page[*tag.WeWorkTag], err error) {

	var tags []*tag.WeWorkTag
	var count int64
	query := this.db.WithContext(this.ctx).
		//Debug().
		Model(tag.WeWorkTag{}).Where(`is_delete = ?`, false)

	if v := option.Option.TagIds; len(v) > 0 {
		query.Where(`tag_id in ?`, v)
	}

	if v := option.Option.GroupIds; len(v) > 0 {
		query.Where(`group_id in ?`, v)
	}
	if v := option.Option.Name; v != `` {
		query.Where(`name like ?`, "%"+v+"%")
	}
	if err := query.Count(&count).Error; err != nil {
		return nil, err
	}
	if option.PageIndex != 0 && option.PageSize != 0 {
		query.Offset((option.PageIndex - 1) * option.PageSize).Limit(option.PageSize)
	}

	err = query.Preload(`WeWorkGroup`).Find(&tags).Error

	return &types.Page[*tag.WeWorkTag]{
		List:      tags,
		PageIndex: option.PageIndex,
		PageSize:  option.PageSize,
		Total:     count,
	}, err

}

// PullListWeWorkCorpTagRequest
//
//	@Description:
//	@receiver this
//	@param tagIds
//	@param groupIds
//	@param sync
//	@return reply
//	@return err
func (this wechatUseCase) PullListWeWorkCorpTagRequest(tagIds []string, groupIds []string, sync int) (reply *response.ResponseTagGetCorpTagList, err error) {

	reply, err = this.wework.ExternalContactTag.GetCorpTagList(this.ctx, tagIds, groupIds)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.pull.wework.crop.tag.error`, reply.ResponseWork)
	}

	if err == nil && sync > 0 {
		// sync to local
		groups, tags := this.transferWeWorkToModel(reply.TagGroups, nil, 0)
		if groups != nil {
			this.modelWeworkTag.group.Action(this.db, groups)
		}
		if tags != nil {
			this.modelWeworkTag.tag.Action(this.db, tags)
		}

	}

	return reply, err

}

// transferWeWorkToModel
//
//	@Description:
//	@receiver this
//	@param data
//	@param agentId
//	@return groups
//	@return tags
func (this wechatUseCase) transferWeWorkToModel(data []*response.CorpTagGroup, agentId *int64, isSelf int) (groups []*tag.WeWorkTagGroup, tags []*tag.WeWorkTag) {

	if data != nil {
		for _, val := range data {
			groups = append(groups, &tag.WeWorkTagGroup{
				PowerModel: powermodel.PowerModel{
					CreatedAt: time.Unix(int64(val.CreateTime), 0),
				},
				//AgentId:  int(*agentId),
				GroupId:  val.GroupID,
				Name:     val.GroupName,
				Sort:     val.Order,
				IsDelete: val.Deleted,
			})
			if val.Tags != nil {
				for _, value := range val.Tags {
					tags = append(tags, &tag.WeWorkTag{
						PowerModel: powermodel.PowerModel{
							CreatedAt: time.Unix(int64(value.CreateTime), 0),
						},
						Type:     1,
						IsSelf:   isSelf,
						TagId:    value.ID,
						GroupId:  val.GroupID,
						Name:     value.Name,
						Sort:     value.Order,
						IsDelete: value.Deleted,
					})
				}

			}
		}
	}
	return groups, tags

}

// PullListWeWorkStrategyTagRequest
//
//	@Description:
//	@receiver this
//	@param options
//	@return reply
//	@return err
func (this wechatUseCase) PullListWeWorkStrategyTagRequest(options *request.RequestTagGetStrategyTagList) (reply *response.ResponseTagGetStrategyTagList, err error) {

	reply, err = this.wework.ExternalContactTag.GetStrategyTagList(this.ctx, options)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.pull.wework.strategy.tag.error`, reply.ResponseWork)
	}
	return reply, err

}

// ActionWeWorkCorpTagGroupRequest
//
//	@Description:
//	@receiver this
//	@param options
//	@return work
//	@return error
func (this *wechatUseCase) ActionWeWorkCorpTagGroupRequest(options *types.ActionCorpTagGroupRequest) (work *baseResp.ResponseWork, err error) {

	//tags := this.modelWeworkTag.tag.FindOneByTagGroupId(this.db, *options.GroupId)
	var addTagGroup []request.RequestTagAddCorpTagFieldTag
	var delTag []string

	for _, newTag := range options.Tags {
		if newTag.TagId == `` {
			addTagGroup = append(addTagGroup, request.RequestTagAddCorpTagFieldTag{
				Name: newTag.TagName,
			})
		} else {
			delTag = append(delTag, newTag.TagId)
		}
	}
	if delTag != nil {
		work, err = this.DeleteWeWorkCorpTagRequest(&request.RequestTagDelCorpTag{TagID: delTag})
	}
	if len(addTagGroup) > 0 {
		add, er := this.CreateWeWorkCorpTagRequest(&request.RequestTagAddCorpTag{
			GroupID:   options.GroupId,
			GroupName: options.GroupName,
			Tag:       addTagGroup,
			AgentID:   options.AgentId,
		})
		err = er
		work = &add.ResponseWork
	}

	return work, err

}

// CreateWeWorkCorpTagRequest
//
//	@Description:
//	@receiver this
//	@param options
//	@return *response.ResponseTagAddCorpTag
//	@return error
func (this *wechatUseCase) CreateWeWorkCorpTagRequest(options *request.RequestTagAddCorpTag) (*response.ResponseTagAddCorpTag, error) {

	corpTag, err := this.wework.ExternalContactTag.AddCorpTag(this.ctx, options)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.create.wework.corp.tag.error`, corpTag.ResponseWork)
	}

	if err == nil {
		groups, tags := this.transferWeWorkToModel([]*response.CorpTagGroup{corpTag.TagGroups}, options.AgentID, 1)
		if groups != nil {
			this.modelWeworkTag.group.Action(this.db, groups)
		}
		if tags != nil {
			this.modelWeworkTag.tag.Action(this.db, tags)
		}
	}

	return corpTag, err

}

// UpdateWeWorkCorpTagRequest
//
//	@Description:
//	@receiver this
//	@param options
//	@return *baseResp.ResponseWork
//	@return error
func (this *wechatUseCase) UpdateWeWorkCorpTagRequest(options *request.RequestTagEditCorpTag) (*baseResp.ResponseWork, error) {

	corpTag, err := this.wework.ExternalContactTag.EditCorpTag(this.ctx, options)

	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.update.wework.corp.tag.error`, *corpTag)
	}
	if err == nil {
		info := this.modelWeworkTag.tag.FindOneByTagId(this.db, options.ID)
		if info != nil {
			info.Name = options.Name
			info.Sort = options.Order
			this.modelWeworkTag.tag.Action(this.db, []*tag.WeWorkTag{info})
		}
	}

	return corpTag, err

}

// DeleteWeWorkCorpTagRequest
//
//	@Description:
//	@receiver this
//	@param options
//	@return *baseResp.ResponseWork
//	@return error
func (this *wechatUseCase) DeleteWeWorkCorpTagRequest(options *request.RequestTagDelCorpTag) (*baseResp.ResponseWork, error) {

	corpTag, err := this.wework.ExternalContactTag.DelCorpTag(this.ctx, options)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.delete.wework.corp.tag.error`, *corpTag)
	}

	err = this.modelWeworkTag.tag.Delete(this.db, options.GroupID, options.TagID)

	return corpTag, err

}

// ActionWeWorkCustomerTagRequest
//
//	@Description:
//	@receiver this
//	@param options
//	@return *baseResp.ResponseWork
//	@return error
func (this *wechatUseCase) ActionWeWorkCustomerTagRequest(option *request.RequestTagMarkTag) (*baseResp.ResponseWork, error) {

	customerTag, err := this.wework.ExternalContactTag.MarkTag(this.ctx, option)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.update.wework.customer.tag.error`, *customerTag)
	}

	if err == nil {
		this.updateCustomerFolowTagIds(option)

	}

	return customerTag, err

}

// updateCustomerFolowTagIds
//
//	@Description:
//	@receiver this
//	@param option
func (this wechatUseCase) updateCustomerFolowTagIds(option *request.RequestTagMarkTag) {

	follow := this.modelWeworkCustomer.follow.FindFollowByExternalUserId(this.db, option.ExternalUserID)
	column := make(map[string]string)
	if follow.TagIds != `` {
		for _, val := range strings.Split(follow.TagIds, `,`) {
			column[val] = val
		}
	}

	if option.AddTag != nil {
		for _, val := range option.AddTag {
			if _, ok := column[val]; !ok {
				column[val] = val
			}
		}
	}
	if option.RemoveTag != nil {
		for _, val := range option.RemoveTag {
			if _, ok := column[val]; ok {
				delete(column, val)
			}
		}
	}
	if column != nil {
		var tagIds []string
		for _, val := range column {
			tagIds = append(tagIds, val)
		}
		follow.TagIds = strings.Join(tagIds, `,`)
		this.modelWeworkCustomer.follow.Action(this.db, []*customer.WeWorkExternalContactFollow{follow})
	}

}
