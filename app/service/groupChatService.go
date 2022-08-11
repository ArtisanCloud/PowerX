package service

import (
	"errors"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/contract"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/power"
	request2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/groupChat/request"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/groupChat/response"
	modelsPowerWechatWork "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/models"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	global2 "github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GroupChatService struct {
	GroupChat *models.GroupChat
}

/**
 ** 初始化构造函数
 */
func NewGroupChatService(ctx *gin.Context) (r *GroupChatService) {
	r = &GroupChatService{
		GroupChat: models.NewGroupChat(nil),
	}
	return r
}

func (srv *GroupChatService) GetSortListForQuery(index int8) string {
	switch index {
	case 1:
		return "chat_id"
	case 2:
		return "wxGroupChatMembers.memberCount"
	case 3:
		return "create_time"
	default:
		return "chat_id"
	}

}

func (srv *GroupChatService) SyncGroupChatFromWXPlatform(statusFilter int, ownerFilter *power.HashMap, cursor string, limit int) (err error) {

	// get all group chat from wx platform
	result, err := srv.GetGroupChatListOnWXPlatform(statusFilter, ownerFilter, cursor, limit)

	if err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return errors.New(result.ErrMSG)
	}

	// sync group chats
	for _, groupChat := range result.GroupChatList {

		responseGroupChat, err := global2.G_WeComApp.App.ExternalContactGroupChat.Get(groupChat.ChatID, 1)
		if err != nil || responseGroupChat.ErrCode != 0 {
			continue
		}
		err = srv.UpsertGroupChats(global.G_DBConnection, modelWX.WX_GROUP_CHAT_UNIQUE_ID, []*models.GroupChat{
			&models.GroupChat{
				PowerCompactModel: databasePowerLib.NewPowerCompactModel(),
				WXGroupChat: &modelWX.WXGroupChat{
					ChatID:     &responseGroupChat.GroupChat.ChatID,
					Name:       &responseGroupChat.GroupChat.Name,
					Owner:      &responseGroupChat.GroupChat.Owner,
					CreateTime: &responseGroupChat.GroupChat.CreateTime,
					Notice:     &responseGroupChat.GroupChat.Notice,
				},
			},
		}, []string{
			"name",
			"owner",
			"create_time",
			"notice",
		})
		if err != nil {
			return err
		}

		// sync group chat members
		for _, member := range responseGroupChat.GroupChat.MemberList {

			invitor, _ := object.JsonEncode(member.Invitor)
			groupChatMember := &modelWX.WXGroupChatMember{
				WXGroupChatID: &responseGroupChat.GroupChat.ChatID,
				UserID:        &member.UserID,
				Type:          &member.Type,
				JoinTime:      &member.JoinTime,
				JoinScene:     &member.JoinScene,
				Invitor:       datatypes.JSON([]byte(invitor)),
				GroupNickName: &member.GroupNickname,
				Name:          &member.Name,
				UnionID:       &member.UnionID,
			}
			groupChatMember.UniqueID = groupChatMember.GetComposedUniqueID()
			err = srv.UpsertGroupChatMembers(global.G_DBConnection, modelWX.WX_GROUP_CHAT_MEMBER_UNIQUE_ID, []*modelWX.WXGroupChatMember{groupChatMember}, []string{
				"wx_group_chat_id",
				"user_id",
				"type",
				"join_time",
				"join_scene",
				"invitor",
				"group_nickname",
				"name",
				"union_id",
			})
			if err != nil {
				return err
			}

		}
		// sync group chat admins
		for _, member := range responseGroupChat.GroupChat.AdminList {
			groupChatAdmin := &modelWX.WXGroupChatAdmin{
				WXGroupChatID: &responseGroupChat.GroupChat.ChatID,
				UserID:        &member.UserID,
			}
			groupChatAdmin.UniqueID = groupChatAdmin.GetComposedUniqueID()
			err = srv.UpsertGroupChatAdmins(global.G_DBConnection, modelWX.WX_GROUP_CHAT_ADMIN_UNIQUE_ID, []*modelWX.WXGroupChatAdmin{groupChatAdmin}, []string{
				"wx_group_chat_id",
				"user_id",
			})
			if err != nil {
				return err
			}

		}
	}
	return nil
}

func (srv *GroupChatService) GetQueryList(db *gorm.DB,
	adminUserID []string,
	name string,
	tagIDs []string,
	sortBy int8,
	ascend bool,
	startDate string,
	status int8,
) (groupChats []*models.GroupChat, err error) {

	groupChats = []*models.GroupChat{}

	subQueryOfGroupChatMemberCount := srv.SubQueryOfGroupChatMemberCount(db)

	db = db.Model(&groupChats).
		//Debug().
		Preload("Tags").
		Preload("WXGroupChatMembers").
		Preload("WXGroupChatAdmins").

		// select result
		Distinct(
			"group_chats.*",
			//"wxGroupChatMembers.*",
			//"rTagToObject.tag_id",
		).

		// Join group chat members
		Joins("LEFT JOIN (?) AS wxGroupChatMembers ON wxGroupChatMembers.wx_group_chat_id = group_chats.chat_id", subQueryOfGroupChatMemberCount).

		// Join group chat admins
		Joins("LEFT JOIN wx_group_chat_admins ON wx_group_chat_admins.wx_group_chat_id = group_chats.chat_id").

		// Join Tags
		Joins("LEFT JOIN r_tag_to_object AS rTagToObject ON group_chats.chat_id = rTagToObject.taggable_object_id").
		Joins("LEFT JOIN tags ON rTagToObject.tag_id = tags.index_tag_id")

	if len(adminUserID) > 0 {
		db.Where("wx_group_chat_admins.user_id IN (?)", adminUserID)
	}

	if len(tagIDs) > 0 {
		db.Where("tags.index_tag_id IN (?)", tagIDs)
	}

	if name != "" {
		db.Where("group_chats.name LIKE ?", "%"+name+"%")
	}

	if startDate != "" {
		startDateUnix := carbon.Parse(startDate).Timestamp()
		db.Where("group_chats.create_time >= ?", startDateUnix)
	}

	if status != 0 {
		db.Where("group_chats.status = ?", status)
	}

	// sort by
	sortFiled := srv.GetSortListForQuery(sortBy)
	OrderType := "ASC"
	if ascend {
		OrderType = "DESC"
	}
	db.Order(sortFiled + " " + OrderType)

	// find result
	result := db.Find(&groupChats)

	if result.Error != nil {
		return nil, result.Error
	}

	return groupChats, err
}

func (srv *GroupChatService) SubQueryOfGroupChatMemberCount(db *gorm.DB) *gorm.DB {

	return db.Model(&modelWX.WXGroupChatMember{}).
		//Debug().
		Select("COUNT(*) AS memberCount, wx_group_chat_id").
		Group("wx_group_chat_id")

}

func (srv *GroupChatService) UpsertGroupChats(db *gorm.DB, uniqueName string, groupChats []*models.GroupChat, fieldsToUpdate []string) error {

	if len(groupChats) <= 0 {
		return nil
	}

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = databasePowerLib.GetModelFields(&models.GroupChat{})
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(fieldsToUpdate),
	}).Create(&groupChats)

	return result.Error
}

func (srv *GroupChatService) UpsertGroupChatMembers(db *gorm.DB, uniqueName string, groupChatMembers []*modelWX.WXGroupChatMember, fieldsToUpdate []string) error {

	if len(groupChatMembers) <= 0 {
		return nil
	}

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = databasePowerLib.GetModelFields(&models.GroupChat{})
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(fieldsToUpdate),
	}).Create(&groupChatMembers)

	return result.Error
}

func (srv *GroupChatService) UpsertGroupChatAdmins(db *gorm.DB, uniqueName string, groupChatAdmins []*modelWX.WXGroupChatAdmin, fieldsToUpdate []string) error {

	if len(groupChatAdmins) <= 0 {
		return nil
	}

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = databasePowerLib.GetModelFields(&models.GroupChat{})
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(fieldsToUpdate),
	}).Create(&groupChatAdmins)

	return result.Error
}

func (srv *GroupChatService) SaveGroupChat(db *gorm.DB, groupChat *models.GroupChat) (*models.GroupChat, error) {

	db = db.Create(groupChat)

	return groupChat, db.Error
}

func (srv *GroupChatService) UpdateGroupChat(db *gorm.DB, groupChat *models.GroupChat, withAssociation bool) (*models.GroupChat, error) {

	if !withAssociation {
		db = db.Omit(clause.Associations)
	}
	db = db.
		//Debug().
		Updates(groupChat)

	return groupChat, db.Error
}

func (srv *GroupChatService) DeleteGroupChatsByChatIDs(db *gorm.DB, configIDs []string) error {
	db = db.
		//Debug().
		Where("chat_id in (?)", configIDs).
		Delete(models.GroupChat{})

	return db.Error
}

func (srv *GroupChatService) DeleteGroupChatByChatID(db *gorm.DB, configID string) error {

	db = db.
		//Debug().
		Where("chat_id", configID).
		Delete(&models.GroupChat{})

	return db.Error
}

func (srv *GroupChatService) GetGroupChatsByChatIDs(db *gorm.DB, configIDs []string) (groupChats []*models.GroupChat, err error) {

	groupChats = []*models.GroupChat{}

	db = db.Where("chat_id in (?)", configIDs)
	result := db.Find(&groupChats)
	return groupChats, result.Error
}

func (srv *GroupChatService) GetGroupChatByChatID(db *gorm.DB, configID string) (groupChat *models.GroupChat, err error) {

	groupChat = &models.GroupChat{}

	preloads := []string{"Tags", "WXGroupChatMembers", "WXGroupChatAdmins"}

	condition := &map[string]interface{}{
		"chat_id": configID,
	}
	err = databasePowerLib.GetFirst(db, condition, groupChat, preloads)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	return groupChat, err
}

func (srv *GroupChatService) GetGroupChatListOnWXPlatform(statusFilter int, ownerFilter *power.HashMap, cursor string, limit int) (groupChats *response.ResponseGroupChatList, err error) {

	if limit <= 0 {
		limit = 1000
	}

	request := &request2.RequestGroupChatList{
		StatusFilter: statusFilter,
		OwnerFilter:  ownerFilter,
		Cursor:       cursor,
		Limit:        limit,
	}

	// get group chat list from wechat platform
	result, err := global2.G_WeComApp.App.ExternalContactGroupChat.List(request)

	return result, err
}

func (srv *GroupChatService) HandleChatCreate(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Chat Create")
	return err
}
func (srv *GroupChatService) HandleChatUpdate(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Chat Update")

	msg := &modelsPowerWechatWork.EventExternalUserUpdateAddMember{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	switch msg.UpdateDetail {
	case modelsPowerWechatWork.CALLBACK_EVENT_UPDATE_DETAIL_ADD_MEMBER:

		break
	case modelsPowerWechatWork.CALLBACK_EVENT_UPDATE_DETAIL_DEL_MEMBER:

		break
	case modelsPowerWechatWork.CALLBACK_EVENT_UPDATE_DETAIL_CHANGE_OWNER:

		break
	case modelsPowerWechatWork.CALLBACK_EVENT_UPDATE_DETAIL_CHANGE_NAME:

		break
	case modelsPowerWechatWork.CALLBACK_EVENT_UPDATE_DETAIL_CHANGE_NOTICE:

		break
	default:

	}

	return err
}
func (srv *GroupChatService) HandleChatDismiss(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Chat Dismiss")
	return err
}
func (srv *GroupChatService) HandleChatDelete(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Chat Delete")
	return err
}
