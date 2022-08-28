package wecom

import (
	"errors"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	object2 "github.com/ArtisanCloud/PowerLibs/v2/object"
	models3 "github.com/ArtisanCloud/PowerSocialite/v2/src/models"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/contract"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/tag/response"
	models2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type WXTagService struct {
	WXTag *wx.WXTag
}

func NewWXTagService(ctx *gin.Context) (r *WXTagService) {
	r = &WXTagService{
		WXTag: wx.NewWXTag(nil),
	}
	return r
}

func (srv *WXTagService) GetList(db *gorm.DB, conditions *map[string]interface{}, page int, pageSize int) (pagination *databasePowerLib.Pagination, err error) {

	arrayWXTags := []*wx.WXTag{}
	pagination, err = databasePowerLib.GetList(db, conditions, &arrayWXTags, nil, page, pageSize)

	return pagination, err
}

func (srv *WXTagService) UpsertWXTagByCorpTag(db *gorm.DB, group *response.CorpTagGroup, tag *response.Tag) (err error) {
	err = srv.UpsertWXTags(db, []*wx.WXTag{
		&wx.WXTag{
			ID:           tag.ID,
			WXTagGroupID: &group.GroupID,
			Name:         &tag.Name,
			CreateTime:   &tag.CreateTime,
			Order:        &tag.Order,
			Deleted:      &tag.Deleted,
		},
	}, []string{
		"wx_tag_group_id",
		"name",
		"create_time",
		"order",
		"deleted",
	})

	return err
}

func (srv *WXTagService) UpsertWXTags(db *gorm.DB, tags []*wx.WXTag, fieldsToUpdate []string) error {

	return databasePowerLib.UpsertModelsOnUniqueID(db, &wx.WXTag{}, wx.WX_TAG_UNIQUE_ID, tags, fieldsToUpdate)
}

func (srv *WXTagService) DeleteWXTagsByGroupIDs(db *gorm.DB, groupIDs []string) error {

	db = db.
		//Debug().
		Where("wx_tag_group_id in (?)", groupIDs).
		Delete(&wx.WXTag{})

	return db.Error
}

func (srv *WXTagService) DeleteWXTagsByIDs(db *gorm.DB, tagIDs []string) error {
	db = db.
		//Debug().
		Where("wx_id in (?)", tagIDs).
		Delete(&wx.WXTag{})

	return db.Error
}

func (srv *WXTagService) DeleteWXTagByID(db *gorm.DB, tagID string) error {
	db = db.
		//Debug().
		Where("wx_id", tagID).
		Delete(&wx.WXTag{})

	return db.Error
}

func (srv *WXTagService) GetWXTags(db *gorm.DB, uuids []string) (tags []*wx.WXTag, err error) {

	tags = []*wx.WXTag{}

	db = db.Where("uuid in (?)", uuids)
	result := db.Find(&tags)
	return tags, result.Error
}

func (srv *WXTagService) GetWXTag(db *gorm.DB, uuid string) (department *wx.WXTag, err error) {

	department = &wx.WXTag{}

	db = db.Scopes(
		databasePowerLib.WhereUUID(uuid),
	)

	result := db.First(department)

	return department, result.Error

}

func (srv *WXTagService) GetWXTagsByIDs(db *gorm.DB, arrayIDs []string) (tags []*wx.WXTag, err error) {
	tags = []*wx.WXTag{}

	if len(arrayIDs) > 0 {
		db = db.
			//Debug().
			Where("wx_id in (?)", arrayIDs).
			Find(&tags)
		err = db.Error
	}

	return tags, err
}

func (srv *WXTagService) SyncWXTagsByFollowInfos(db *gorm.DB, pivot *models.RCustomerToEmployee, followInfo *models3.FollowUser) (err error) {
	tagIDs := []string{}

	if len(followInfo.Tags) > 0 {
		for _, tag := range followInfo.Tags {
			tagIDs = append(tagIDs, tag.TagID)
		}
	} else if len(followInfo.TagIDs) > 0 {
		for _, tag := range followInfo.Tags {
			tagIDs = append(tagIDs, tag.TagID)
		}
	}

	// sync wechat tags to custmer and employee pivot table
	tags, _ := srv.GetWXTagsByIDs(db, tagIDs)
	err = srv.SyncWXTagsToObject(db, pivot, tags)
	if err != nil {
		fmt.Dump(err.Error())
	}
	return nil
}

// ---------------------------------------------------------
func (srv *WXTagService) AppendWXTagsToObject(db *gorm.DB, obj databasePowerLib.ModelInterface, tags []*wx.WXTag) (err error) {

	pivots, err := (&wx.RWXTagToObject{}).MakePivotsFromObjectAndTags(obj, tags)
	if err != nil {
		return err
	}
	err = databasePowerLib.AppendMorphPivots(db, pivots)
	return err
}

func (srv *WXTagService) SyncWXTagsToObject(db *gorm.DB, obj databasePowerLib.ModelInterface, tags []*wx.WXTag) (err error) {

	if tags == nil || len(tags) == 0 {
		return nil
	}
	pivots, err := (&wx.RWXTagToObject{}).MakePivotsFromObjectAndTags(obj, tags)
	if err != nil {
		return err
	}
	err = databasePowerLib.SyncMorphPivots(db, pivots)
	return err
}

func (srv *WXTagService) ClearObjectWXTags(db *gorm.DB, obj databasePowerLib.ModelInterface) (err error) {
	err = databasePowerLib.ClearPivots(db, &wx.RWXTagToObject{
		TaggableOwnerType: object2.NewNullString(obj.GetTableName(true), true),
		TaggableObjectID:  object2.NewNullString(obj.GetForeignReferValue(), true),
	}, true, false)
	return err
}

// ------------------------------------------------------------
func (srv *WXTagService) HandleTagCreate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &models2.EventExternalUserTagCreate{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	tagIDs := []string{}
	groupIDs := []string{}
	if msg.TagType == models2.CALLBACK_EVENT_TAG_TYPE {
		tagIDs = append(tagIDs, msg.ID)
	} else if msg.TagType == models2.CALLBACK_EVENT_TAG_TYPE_GROUP {
		groupIDs = append(groupIDs, msg.ID)
	} else {
		return errors.New("Unsupported tag type")
	}

	logger.Logger.Info("Handle tag create", zap.Any("msg", msg), zap.Any("tagIDs", tagIDs), zap.Any("groupIDs", groupIDs))
	serviceTagGroup := NewWXTagGroupService(context)
	err = serviceTagGroup.SyncWXTagGroupsFromWXPlatform(tagIDs, groupIDs, false)

	return err

}
func (srv *WXTagService) HandleTagUpdate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &models2.EventExternalUserTagUpdate{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	tagIDs := []string{}
	groupIDs := []string{}
	if msg.TagType == models2.CALLBACK_EVENT_TAG_TYPE {
		tagIDs = append(tagIDs, msg.ID)
	} else if msg.TagType == models2.CALLBACK_EVENT_TAG_TYPE_GROUP {
		groupIDs = append(groupIDs, msg.ID)
	} else {
		return errors.New("Unsupported tag type")
	}

	logger.Logger.Info("Handle tag update", zap.Any("msg", msg), zap.Any("tagIDs", tagIDs), zap.Any("groupIDs", groupIDs))
	serviceTagGroup := NewWXTagGroupService(context)
	err = serviceTagGroup.SyncWXTagGroupsFromWXPlatform(tagIDs, groupIDs, false)

	return err
}

func (srv *WXTagService) HandleTagDelete(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &models2.EventExternalUserTagDelete{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	tagIDs := []string{}
	groupIDs := []string{}
	if msg.TagType == models2.CALLBACK_EVENT_TAG_TYPE {
		tagIDs = append(tagIDs, msg.ID)
	} else if msg.TagType == models2.CALLBACK_EVENT_TAG_TYPE_GROUP {
		groupIDs = append(groupIDs, msg.ID)
	} else {
		return errors.New("Unsupported tag type")
	}

	logger.Logger.Info("Handle tag delete", zap.Any("msg", msg), zap.Any("tagIDs", tagIDs), zap.Any("groupIDs", groupIDs))
	serviceTagGroup := NewWXTagGroupService(context)
	err = serviceTagGroup.DeleteWXTagGroups(tagIDs, groupIDs)

	return err
}

func (srv *WXTagService) HandleTagShuffle(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &models2.EventExternalUserTagShuffle{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	groupIDs := []string{}
	if msg.ID != "" {
		groupIDs = append(groupIDs, msg.ID)
	}

	logger.Logger.Info("Handle tag shuffle", zap.Any("msg", msg), zap.Any("groupIDs", groupIDs))
	logger.Logger.Info("do nothing shuffle")
	//serviceTagGroup := NewWXTagGroupService(context)
	//err = serviceTagGroup.SyncWXTagGroupsFromWXPlatform([]string{}, groupIDs)

	return err
}
