package service

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/contract"
	request2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/contactWay/request"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/contactWay/response"
	models2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ContactWayService struct {
	ContactWay *models.ContactWay
}

/**
 ** 初始化构造函数
 */
func NewContactWayService(ctx *gin.Context) (r *ContactWayService) {
	r = &ContactWayService{
		ContactWay: models.NewContactWay(nil),
	}
	return r
}

func (srv *ContactWayService) SyncContactWayFromWXPlatform(startDatetime *carbon.Carbon, endDatetime *carbon.Carbon, limit int) (err error) {

	// get all contact way from wx platform
	result, err := srv.GetContactWayListOnWXPlatform(startDatetime, endDatetime, limit)

	if err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return errors.New(result.ErrMSG)
	}

	// sync contact ways
	for _, contactWayID := range result.ContactWayIDs {

		responseContactWay, err := wecom.G_WeComEmployee.App.ExternalContactContactWay.Get(contactWayID.ConfigID)

		users, _ := object.JsonEncode(responseContactWay.ContactWay.User)
		parties, _ := object.JsonEncode(responseContactWay.ContactWay.Party)
		conclusions, _ := object.JsonEncode(responseContactWay.ContactWay.Conclusions)

		err = srv.UpsertContactWays(global.G_DBConnection, []*models.ContactWay{
			&models.ContactWay{
				PowerModel: database.NewPowerModel(),
				CodeURL:    responseContactWay.ContactWay.QrCode,
				WXContactWay: &wx.WXContactWay{
					ConfigID:      contactWayID.ConfigID,
					Type:          &responseContactWay.ContactWay.Type,
					Scene:         &responseContactWay.ContactWay.Scene,
					Style:         &responseContactWay.ContactWay.Style,
					Remark:        &responseContactWay.ContactWay.Remark,
					SkipVerify:    &responseContactWay.ContactWay.SkipVerify,
					State:         &responseContactWay.ContactWay.State,
					User:          datatypes.JSON([]byte(users)),
					Party:         datatypes.JSON([]byte(parties)),
					IsTemp:        &responseContactWay.ContactWay.IsTemp,
					ExpiresIn:     &responseContactWay.ContactWay.ExpiresIn,
					ChatExpiresIn: &responseContactWay.ContactWay.ChatExpiresIn,
					UnionID:       &responseContactWay.ContactWay.UnionID,
					Conclusions:   datatypes.JSON([]byte(conclusions)),
				},
			},
		}, []string{
			"type",
			"scene",
			"style",
			"remark",
			"skip_verify",
			"state",
			"user",
			"party",
			"is_temp",
			"expires_in",
			"chat_expires_in",
			"union_id",
			"conclusions",
		})
		if err != nil {
			return err
		}

	}
	return nil
}

func (srv *ContactWayService) GetList(db *gorm.DB, groupUUID string) (contactWays []*models.ContactWay, err error) {

	contactWays = []*models.ContactWay{}

	var conditions *map[string]interface{}
	if groupUUID != "" {
		conditions = &map[string]interface{}{
			"group_uuid": groupUUID,
		}
	}

	err = database.GetAllList(db, conditions, &contactWays, []string{"WXTags"})

	return contactWays, err
}

func (srv *ContactWayService) UpsertContactWays(db *gorm.DB, contactWays []*models.ContactWay, fieldsToUpdate []string) error {

	return database.UpsertModelsOnUniqueID(db, &models.ContactWay{}, wx.WX_CONTACT_WAY_UNIQUE_ID, contactWays, fieldsToUpdate)
}

func (srv *ContactWayService) SaveContactWay(db *gorm.DB, contactWay *models.ContactWay) (*models.ContactWay, error) {

	db = db.Create(contactWay)

	return contactWay, db.Error
}

func (srv *ContactWayService) UpdateContactWay(db *gorm.DB, contactWay *models.ContactWay, withAssociation bool) (*models.ContactWay, error) {
	db = db
	if !withAssociation {
		db = db.Omit(clause.Associations)
	}
	db = db.
		//Debug().
		Updates(contactWay)

	return contactWay, db.Error
}

func (srv *ContactWayService) DeleteContactWaysByConfigIDs(db *gorm.DB, configIDs []string) error {

	db = db.
		//Debug().
		Where("config_id in (?)", configIDs).
		Delete(models.ContactWay{})

	return db.Error
}

func (srv *ContactWayService) DeleteContactWayByConfigID(db *gorm.DB, configID string) error {

	db = db.
		//Debug().
		Where("config_id", configID).
		Delete(&models.ContactWay{})

	return db.Error
}

func (srv *ContactWayService) GetContactWaysByConfigIDs(db *gorm.DB, configIDs []string) (contactWays []*models.ContactWay, err error) {

	contactWays = []*models.ContactWay{}

	db = db.Where("config_id in (?)", configIDs)
	result := db.Find(&contactWays)
	return contactWays, result.Error
}

func (srv *ContactWayService) GetContactWayByConfigID(db *gorm.DB, configID string) (contactWay *models.ContactWay, err error) {

	contactWay = &models.ContactWay{}

	condition := &map[string]interface{}{
		"config_id": configID,
	}
	err = database.GetFirst(db, condition, contactWay, nil)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	return contactWay, err
}

func (srv *ContactWayService) GetContactWayByState(db *gorm.DB, state string) (contactWay *models.ContactWay, err error) {

	contactWay = &models.ContactWay{}

	condition := &map[string]interface{}{
		"state": state,
	}
	err = database.GetFirst(db, condition, contactWay, nil)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return contactWay, err
}

func (srv *ContactWayService) DetachContactWayGroup(db *gorm.DB, groupUUIDs []string) (err error) {

	db = db.Model(&models.ContactWay{}).
		//Debug().
		Where("group_uuid in (?)", groupUUIDs).
		Update("group_uuid", nil)

	return db.Error
}

func (srv *ContactWayService) GetContactWayListOnWXPlatform(startDatetime *carbon.Carbon, endDatetime *carbon.Carbon, limit int) (contactWays *response.ResponseListContactWay, err error) {

	start := carbon.Now().AddDays(-1).Carbon2Time().Unix()
	if startDatetime != nil && startDatetime.IsZero() == false {
		start = startDatetime.Carbon2Time().Unix()
	}

	end := carbon.Now().Carbon2Time().Unix()
	if endDatetime != nil && endDatetime.IsZero() == false {
		end = endDatetime.Carbon2Time().Unix()
	}

	if limit <= 0 {
		limit = 1000
	}

	request := &request2.RequestListContactWay{
		StartTime: start,
		EndTime:   end,
		Cursor:    "",
		Limit:     limit,
	}

	// get tag group list from wechat platform
	result, err := wecom.G_WeComEmployee.App.ExternalContactContactWay.List(request)

	return result, err
}

func (srv *ContactWayService) CreateContactWayOnWXPlatform(contactWay *models.ContactWay) (result *response.ResponseAddContactWay, err error) {

	users := []string{}
	err = object.JsonDecode(contactWay.WXContactWay.User, &users)
	if err != nil {
		return nil, err
	}
	parties := []int{}
	err = object.JsonDecode(contactWay.WXContactWay.Party, &parties)
	if err != nil {
		return nil, err
	}

	conclusions := models.NewConclusions()
	err = object.JsonDecode(contactWay.WXContactWay.Conclusions, conclusions)
	if err != nil {
		return nil, err
	}

	request := &request2.RequestAddContactWay{
		Type:          *contactWay.WXContactWay.Type,
		Scene:         *contactWay.WXContactWay.Scene,
		Style:         *contactWay.WXContactWay.Style,
		Remark:        *contactWay.WXContactWay.Remark,
		SkipVerify:    *contactWay.WXContactWay.SkipVerify,
		State:         *contactWay.WXContactWay.State,
		User:          users,
		Party:         parties,
		IsTemp:        *contactWay.WXContactWay.IsTemp,
		ExpiresIn:     *contactWay.WXContactWay.ExpiresIn,
		ChatExpiresIn: *contactWay.WXContactWay.ChatExpiresIn,
		//UnionID:       *contactWay.WXContactWay.UnionID,
		Conclusions: conclusions,
	}

	result, err = wecom.G_WeComEmployee.App.ExternalContactContactWay.Add(request)

	if err != nil {
		return nil, err
	}

	if result.ErrCode != 0 {
		return result, errors.New(result.ErrMSG)
	}

	return result, nil
}

func (srv *ContactWayService) UpdateContactWayOnWXPlatform(contactWay *models.ContactWay) (err error) {

	users := []string{}
	err = object.JsonDecode(contactWay.WXContactWay.User, &users)
	if err != nil {
		return err
	}
	parties := []int{}
	err = object.JsonDecode(contactWay.WXContactWay.Party, &parties)
	if err != nil {
		return err
	}

	conclusions := models.NewConclusions()
	err = object.JsonDecode(contactWay.WXContactWay.Conclusions, conclusions)
	if err != nil {
		return err
	}

	request := &request2.RequestUpdateContactWay{
		ConfigID:      contactWay.ConfigID,
		Remark:        *contactWay.WXContactWay.Remark,
		SkipVerify:    *contactWay.WXContactWay.SkipVerify,
		Style:         *contactWay.WXContactWay.Style,
		State:         *contactWay.WXContactWay.State,
		User:          users,
		Party:         parties,
		ExpiresIn:     *contactWay.WXContactWay.ExpiresIn,
		ChatExpiresIn: *contactWay.WXContactWay.ChatExpiresIn,
		//UnionID:       contactWay.WXContactWay.UnionID,
		Conclusions: conclusions,
	}

	result, err := wecom.G_WeComEmployee.App.ExternalContactContactWay.Update(request)

	if err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return errors.New(result.ErrMSG)
	}

	return nil
}

func (srv *ContactWayService) DeleteContactWayOnWXPlatform(configID string) (err error) {

	result, err := wecom.G_WeComEmployee.App.ExternalContactContactWay.Delete(configID)

	if err != nil {
		return err
	}

	if result.ErrCode != 0 {
		//return
		logger.Logger.Error(result.ErrMSG)
	}

	return nil
}

func (srv *ContactWayService) ConvertResponseToContactWay(contactWay *models.ContactWay, responseContactWay *response.ResponseAddContactWay) (*models.ContactWay, error) {

	contactWay.ConfigID = responseContactWay.ConfigID
	contactWay.CodeURL = responseContactWay.QRCode

	return contactWay, nil
}

// ---------------------------------------------------------------------------------------------------------------------

func (srv *ContactWayService) HandleChatCreate(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Chat Create")
	return err
}
func (srv *ContactWayService) HandleChatUpdate(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Chat Update")

	msg := &models2.EventExternalUserUpdateAddMember{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	switch msg.UpdateDetail {
	case models2.CALLBACK_EVENT_UPDATE_DETAIL_ADD_MEMBER:

		break
	case models2.CALLBACK_EVENT_UPDATE_DETAIL_DEL_MEMBER:

		break
	case models2.CALLBACK_EVENT_UPDATE_DETAIL_CHANGE_OWNER:

		break
	case models2.CALLBACK_EVENT_UPDATE_DETAIL_CHANGE_NAME:

		break
	case models2.CALLBACK_EVENT_UPDATE_DETAIL_CHANGE_NOTICE:

		break
	default:

	}

	return err
}
func (srv *ContactWayService) HandleChatDismiss(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Chat Dismiss")
	return err
}
func (srv *ContactWayService) HandleChatDelete(context *gin.Context, event contract.EventInterface) (err error) {
	fmt.Dump("Handle Chat Delete")
	return err
}
