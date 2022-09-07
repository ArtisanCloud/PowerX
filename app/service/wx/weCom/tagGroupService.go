package weCom

import (
	"errors"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	request2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/tag/request"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/tag/response"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WXTagGroupService struct {
	*WeComService
	WXTagGroup *wx.WXTagGroup
}

func NewWXTagGroupService(ctx *gin.Context) (r *WXTagGroupService) {
	r = &WXTagGroupService{
		WeComService: G_WeComEmployee,
		WXTagGroup:   wx.NewWXTagGroup(nil),
	}
	return r
}

func (srv *WXTagGroupService) GetList(db *gorm.DB, wxDepartmentID int) (arrayWXTagGroups []*wx.WXTagGroup, err error) {

	arrayWXTagGroups = []*wx.WXTagGroup{}

	var conditions *map[string]interface{} = nil
	if wxDepartmentID > 0 {
		conditions = &map[string]interface{}{
			"wx_department_id": wxDepartmentID,
		}
	}

	err = databasePowerLib.GetAllList(db, conditions, &arrayWXTagGroups, []string{"WXTags"})

	return arrayWXTagGroups, err
}

func (srv *WXTagGroupService) UpsertWXTagGroupsByCorpTagGroup(db *gorm.DB, group *response.CorpTagGroup) (err error) {
	err = srv.UpsertWXTagGroups(db, []*wx.WXTagGroup{
		&wx.WXTagGroup{
			GroupID:    group.GroupID,
			GroupName:  &group.GroupName,
			CreateTime: &group.CreateTime,
			Order:      &group.Order,
			Deleted:    &group.Deleted,
		},
	}, []string{
		"group_name",
		"create_time",
		"order",
		"deleted",
	})

	return err
}

func (srv *WXTagGroupService) UpsertWXTagGroups(db *gorm.DB, tagGroups []*wx.WXTagGroup, fieldsToUpdate []string) error {

	return databasePowerLib.UpsertModelsOnUniqueID(db, &wx.WXTagGroup{}, wx.WX_TAG_GROUP_UNIQUE_ID, tagGroups, fieldsToUpdate)
}

func (srv *WXTagGroupService) DeleteWXTagGroups(tagIDs []string, groupIDs []string) (err error) {
	err = global.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		serviceWXTag := NewWXTagService(nil)
		if len(groupIDs) > 0 {
			err = serviceWXTag.DeleteWXTagsByGroupIDs(global.G_DBConnection, groupIDs)
			if err != nil {
				return err
			}
			err = srv.DeleteWXTagGroupsByIDs(global.G_DBConnection, groupIDs)
			if err != nil {
				return err
			}
		}

		if len(tagIDs) > 0 {
			err = serviceWXTag.DeleteWXTagsByIDs(global.G_DBConnection, tagIDs)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

func (srv *WXTagGroupService) DeleteWXTagGroupsByIDs(db *gorm.DB, tagGroupIDs []string) error {

	db = db.
		//Debug().
		Where("group_id in (?)", tagGroupIDs).
		Delete(&wx.WXTagGroup{})

	return db.Error
}

func (srv *WXTagGroupService) DeleteWXTagGroupByID(db *gorm.DB, tagGroupID *string) error {

	db = db.
		//Debug().
		Where("group_id", tagGroupID).
		Delete(&wx.WXTagGroup{})

	return db.Error
}

func (srv *WXTagGroupService) GetWXTagGroups(db *gorm.DB, ids []string) (tags []*wx.WXTagGroup, err error) {

	tags = []*wx.WXTagGroup{}

	db = db.Where("group_id in (?)", ids)
	result := db.Find(&tags)
	return tags, result.Error
}

func (srv *WXTagGroupService) GetWXTagGroup(db *gorm.DB, id string) (department *wx.WXTagGroup, err error) {

	department = &wx.WXTagGroup{}

	db = db.Preload("Tags").Where("group_id = ?", id)

	result := db.
		//Debug().
		First(department)

	return department, result.Error

}

func (srv *WXTagGroupService) GetWXTagGroupsByIDs(db *gorm.DB, arrayIDs []int) (tags []*wx.WXTagGroup, err error) {
	tags = []*wx.WXTagGroup{}

	if len(arrayIDs) > 0 {
		db = db.Table(wx.TABLE_NAME_TAG_GROUP).Where("id in (?)", arrayIDs).Find(&tags)
		err = db.Error
	}

	return tags, err
}

func (srv *WXTagGroupService) SyncWXTagGroupsFromWXPlatform(tagIDs []string, groupIDs []string, needForceSync bool) (err error) {

	// get tag group list from wechat platform
	result, err := G_WeComApp.App.ExternalContactTag.GetCorpTagList(tagIDs, groupIDs)

	if err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return errors.New(result.ErrMSG)
	}

	err = global.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		serviceWXTag := NewWXTagService(nil)
		// sync tag groups
		for _, group := range result.TagGroups {

			// sync tag group
			err = srv.UpsertWXTagGroupsByCorpTagGroup(tx, group)
			if err != nil {
				return err
			}

			// clear tags, if user need force sync
			if needForceSync {
				err = serviceWXTag.DeleteWXTagsByGroupIDs(tx, []string{group.GroupID})
				if err != nil {
					return err
				}
			}

			// upsert tags
			for _, tag := range group.Tags {
				err = serviceWXTag.UpsertWXTagByCorpTag(tx, group, tag)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})

	return err
}

func (srv *WXTagGroupService) CreateWXTagGroupOnWXPlatform(requestTagGroup *wx.WXTagGroup, agentID *int64) (result *response.ResponseTagAddCorpTag, err error) {
	tags := []request2.RequestTagAddCorpTagFieldTag{}
	for _, tag := range requestTagGroup.WXTags {
		newTag := request2.RequestTagAddCorpTagFieldTag{
			Name:  *tag.Name,
			Order: *tag.Order,
		}
		tags = append(tags, newTag)
	}

	var groupID *string = nil

	if requestTagGroup.GroupID != "" {
		groupID = &requestTagGroup.GroupID
	}

	request := &request2.RequestTagAddCorpTag{
		GroupID:   groupID,
		GroupName: *requestTagGroup.GroupName,
		Order:     *requestTagGroup.Order,
		Tag:       tags,
		AgentID:   agentID,
	}

	result, err = G_WeComApp.App.ExternalContactTag.AddCorpTag(request)

	if err != nil {
		return nil, err
	}

	if result.ErrCode != 0 {
		return result, errors.New(result.ErrMSG)
	}

	return result, nil
}

func (srv *WXTagGroupService) UpdateWXTagGroupOnWXPlatform(requestTagGroup *wx.WXTagGroup, agentID *int64) (err error) {

	request := &request2.RequestTagEditCorpTag{
		ID:      requestTagGroup.GroupID,
		Name:    *requestTagGroup.GroupName,
		Order:   *requestTagGroup.Order,
		AgentID: agentID,
	}

	result, err := G_WeComApp.App.ExternalContactTag.EditCorpTag(request)

	if err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return errors.New(result.ErrMSG)
	}

	return nil
}

func (srv *WXTagGroupService) DeleteWXTagGroupOnWXPlatform(groupIDs []string, tagsIDs []string, agentID *int64) (err error) {

	request := &request2.RequestTagDelCorpTag{
		GroupID: groupIDs,
		TagID:   tagsIDs,
		AgentID: agentID,
	}

	result, err := G_WeComApp.App.ExternalContactTag.DelCorpTag(request)

	if err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return errors.New(result.ErrMSG)
	}

	return nil
}

func (srv *WXTagGroupService) ConvertResponseToWXTagGroup(responseTagGroup *response.ResponseTagAddCorpTag, departmentID *int) (wxTagGroup *wx.WXTagGroup, err error) {

	wxTagGroup = wx.NewWXTagGroup(nil)
	wxTagGroup.WXDepartmentID = departmentID
	wxTagGroup.GroupID = responseTagGroup.TagGroups.GroupID
	wxTagGroup.GroupName = &responseTagGroup.TagGroups.GroupName
	wxTagGroup.CreateTime = &responseTagGroup.TagGroups.CreateTime
	wxTagGroup.Order = &responseTagGroup.TagGroups.Order
	wxTagGroup.Deleted = &responseTagGroup.TagGroups.Deleted

	for _, tag := range responseTagGroup.TagGroups.Tags {

		newTag := wx.NewWXTag(nil)
		newTag.WXDepartmentID = departmentID
		newTag.TempID = nil
		newTag.ID = tag.ID
		newTag.Name = &tag.Name
		newTag.CreateTime = &tag.CreateTime
		newTag.Order = &tag.Order
		newTag.Deleted = &tag.Deleted
		newTag.WXTagGroupID = &responseTagGroup.TagGroups.GroupID

		wxTagGroup.WXTags = append(wxTagGroup.WXTags, newTag)

	}

	return wxTagGroup, nil
}
