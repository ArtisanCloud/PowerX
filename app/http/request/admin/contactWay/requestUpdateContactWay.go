package contactWay

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	request2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/contactWay/request"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service"
	serviceWX "github.com/ArtisanCloud/PowerX/app/service/wx/weCom"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type ParaUpdateContactWay struct {
	ConfigID string `form:"configID" json:"configID" xml:"configID"`

	Name                            string                `form:"name" json:"name" binding:"required"`
	GroupUUID                       string                `form:"groupUUID" json:"groupUUID" binding:"required"`
	AllowEmployeeChangeOnlineStatus bool                  `form:"allowEmployeeChangeOnlineStatus" json:"allowEmployeeChangeOnlineStatus" binding:"required"`
	RemarkAccount                   string                `form:"remarkAccount" json:"remarkAccount"`
	RemarkAccountPrefix             bool                  `form:"remarkAccountPrefix" json:"remarkAccountPrefix"`
	WelcomeMessageType              int8                  `form:"welcomeMessageType" json:"welcomeMessageType"`
	Type                            int                   `form:"type" json:"type"`
	Scene                           int                   `form:"scene" json:"scene"`
	Style                           int                   `form:"style" json:"style"`
	Remark                          string                `form:"remark" json:"remark"`
	SkipVerify                      bool                  `form:"skipVerify" json:"skipVerify"`
	User                            []string              `form:"user" json:"user"`
	Party                           []int                 `form:"party" json:"party"`
	Conclusions                     *request2.Conclusions `form:"conclusions" json:"conclusions"`
	WXTagIDs                        []string              `form:"wxTagIDs" json:"wxTagIDs"`
	//IsTemp        bool                  `form:"isTemp" json:"isTemp"`
	//ExpiresIn     int                   `form:"expiresIn" json:"expiresIn"`
	//ChatExpiresIn int                   `form:"chatExpiresIn" json:"chatExpiresIn"`
	//UnionID       string                `form:"unionID" json:"unionID"`
}

func ValidateUpdateContactWay(context *gin.Context) {
	var form ParaUpdateContactWay

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	contactWay, updateTags, err := convertParaToContactWayForUpdate(form)
	if err != nil {
		return
	}

	context.Set("contactWay", contactWay)
	context.Set("updateTags", updateTags)
	context.Next()
}

func convertParaToContactWayForUpdate(form ParaUpdateContactWay) (contactWay *models.ContactWay, updateTags []*wx.WXTag, err error) {

	contactWayService := service.NewContactWayService(nil)
	contactWay, err = contactWayService.GetContactWayByConfigID(global.G_DBConnection, form.ConfigID)
	if err != nil {
		return nil, nil, err
	}

	users, err := object.JsonEncode(form.User)
	if err != nil {
		return nil, nil, err
	}
	parties, err := object.JsonEncode(form.Party)
	if err != nil {
		return nil, nil, err
	}
	conclusions, err := object.JsonEncode(form.Conclusions)
	if err != nil {
		return nil, nil, err
	}

	contactWay.Name = form.Name
	contactWay.GroupUUID = form.GroupUUID
	contactWay.AllowEmployeeChangeOnlineStatus = form.AllowEmployeeChangeOnlineStatus
	contactWay.RemarkAccount = form.RemarkAccount
	contactWay.RemarkAccountPrefix = form.RemarkAccountPrefix
	contactWay.WelcomeMessageType = form.WelcomeMessageType

	contactWay.WXContactWay.Type = &form.Type
	contactWay.WXContactWay.Scene = &form.Scene
	contactWay.WXContactWay.Style = &form.Style
	contactWay.WXContactWay.Remark = &form.Remark
	contactWay.WXContactWay.SkipVerify = &form.SkipVerify
	contactWay.WXContactWay.User = datatypes.JSON([]byte(users))
	contactWay.WXContactWay.Party = datatypes.JSON([]byte(parties))
	contactWay.WXContactWay.Conclusions = datatypes.JSON([]byte(conclusions))

	// load wxTagIDs
	wxTagService := serviceWX.NewWXTagService(nil)
	updateTags, err = wxTagService.GetWXTagsByIDs(global.G_DBConnection, form.WXTagIDs)
	if err != nil {
		return nil, nil, err
	}

	return contactWay, updateTags, nil
}
