package contactWay

import (
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	request2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/contactWay/request"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	serviceWX "github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type ParaCreateContactWay struct {
	Name                            string                `form:"name" json:"name" binding:"required"`
	GroupUUID                       string                `form:"groupUUID" json:"groupUUID" binding:"required"`
	AllowEmployeeChangeOnlineStatus bool                  `form:"allowEmployeeChangeOnlineStatus" json:"allowEmployeeChangeOnlineStatus"`
	RemarkAccount                   string                `form:"remarkAccount" json:"remarkAccount"`
	RemarkAccountPrefix             bool                  `form:"remarkAccountPrefix" json:"remarkAccountPrefix"`
	WelcomeMessageType              int8                  `form:"welcomeMessageType" json:"welcomeMessageType"`
	Type                            int                   `form:"type" json:"type"`
	Scene                           int                   `form:"scene" json:"scene"`
	Style                           int                   `form:"style" json:"style"`
	Remark                          string                `form:"remark" json:"remark"`
	SkipVerify                      bool                  `form:"skipVerify" json:"skipVerify"`
	State                           string                `form:"state" json:"state"`
	User                            []string              `form:"user" json:"user"`
	Party                           []int                 `form:"party" json:"party"`
	Conclusions                     *request2.Conclusions `form:"conclusions" json:"conclusions"`
	Attachments                     []*object.HashMap     `form:"attachments" json:"attachments"`
	WXTagIDs                        []string              `form:"wxTagIDs" json:"wxTagIDs"`
	//IsTemp        bool                  `form:"isTemp" json:"isTemp"`
	//ExpiresIn     int                   `form:"expiresIn" json:"expiresIn"`
	//ChatExpiresIn int                   `form:"chatExpiresIn" json:"chatExpiresIn"`
	//UnionID       string                `form:"unionID" json:"unionID"`
}

func ValidateCreateContactWay(context *gin.Context) {
	var form ParaCreateContactWay
	apiResponse := http.NewAPIResponse(context)

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	contactWay, err := convertParaToContactWayForCreate(form)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).
			ThrowJSONResponse(context)
		return
	}

	context.Set("contactWay", contactWay)
	context.Next()
}

func convertParaToContactWayForCreate(form ParaCreateContactWay) (contactWay *models.ContactWay, err error) {

	var configID string = ""

	users, err := object.JsonEncode(form.User)
	if err != nil {
		return nil, err
	}
	parties, err := object.JsonEncode(form.Party)
	if err != nil {
		return nil, err
	}
	conclusions, err := object.JsonEncode(form.Conclusions)
	if err != nil {
		return nil, err
	}
	attachments, err := object.JsonEncode(form.Attachments)
	if err != nil {
		return nil, err
	}

	isTemp := false
	expiresIn := 7 * config.DAY
	chatExpiresIn := 24 * config.HOUR
	state := object.RandStringBytesMask(30)
	contactWay = &models.ContactWay{
		PowerModel:                      databasePowerLib.NewPowerModel(),
		Name:                            form.Name,
		GroupUUID:                       form.GroupUUID,
		AllowEmployeeChangeOnlineStatus: form.AllowEmployeeChangeOnlineStatus,
		RemarkAccount:                   form.RemarkAccount,
		RemarkAccountPrefix:             form.RemarkAccountPrefix,
		WelcomeMessageType:              form.WelcomeMessageType,
		Attachments:                     datatypes.JSON([]byte(attachments)),
		Status:                          databasePowerLib.MODEL_STATUS_ACTIVE,

		WXContactWay: &wx.WXContactWay{
			ConfigID:      configID,
			Type:          &form.Type,
			Scene:         &form.Scene,
			Style:         &form.Style,
			Remark:        &form.Remark,
			SkipVerify:    &form.SkipVerify,
			State:         &state,
			User:          datatypes.JSON([]byte(users)),
			Party:         datatypes.JSON([]byte(parties)),
			IsTemp:        &isTemp,
			ExpiresIn:     &expiresIn,
			ChatExpiresIn: &chatExpiresIn,
			Conclusions:   datatypes.JSON([]byte(conclusions)),
			//UnionID:       &form.UnionID,
		},
	}

	// load wxTagIDs
	wxTagService := serviceWX.NewWXTagService(nil)
	wxTags, err := wxTagService.GetWXTagsByIDs(global.G_DBConnection, form.WXTagIDs)
	if err != nil {
		return nil, err
	}

	contactWay.WXTags = wxTags

	return contactWay, nil
}
