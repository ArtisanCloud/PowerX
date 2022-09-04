package admin

import (
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ContactWayGroupAPIController struct {
	*api.APIController
	ServiceContactWayGroup *service.ContactWayGroupService
}

func NewContactWayGroupAPIController(context *gin.Context) (ctl *ContactWayGroupAPIController) {

	return &ContactWayGroupAPIController{
		APIController:          api.NewAPIController(context),
		ServiceContactWayGroup: service.NewContactWayGroupService(context),
	}
}

func APIGetContactWayGroupList(context *gin.Context) {
	ctl := NewContactWayGroupAPIController(context)

	defer api.RecoverResponse(context, "api.admin.contactWayGroup.list")

	arrayList, err := ctl.ServiceContactWayGroup.GetList(globalDatabase.G_DBConnection, nil)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_CONTACT_WAY_GROUP_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetContactWayGroupDetail(context *gin.Context) {
	ctl := NewContactWayGroupAPIController(context)

	params, _ := context.Get("params")
	para := params.(request.ParaDetail)
	fmt.Dump(para)
	defer api.RecoverResponse(context, "api.admin.contactWayGroup.detail")

	contactWayGroup, err := ctl.ServiceContactWayGroup.GetContactWayGroup(globalDatabase.G_DBConnection, para.UUID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_CONTACT_WAY_GROUP_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	_, err = contactWayGroup.LoadContactWays(globalDatabase.G_DBConnection, nil)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_CONTACT_WAY_GROUP_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, contactWayGroup)
}

func APIUpsertContactWayGroup(context *gin.Context) {
	ctl := NewContactWayGroupAPIController(context)

	params, _ := context.Get("contactWayGroup")
	contactWayGroup := params.(*models.ContactWayGroup)

	defer api.RecoverResponse(context, "api.admin.contactWayGroup.upsert")

	var err error
	contactWayGroup, err = ctl.ServiceContactWayGroup.UpsertContactWayGroup(globalDatabase.G_DBConnection, contactWayGroup, false)

	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_CONTACT_WAY_GROUP, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, contactWayGroup)

}

func APIDeleteContactWayGroups(context *gin.Context) {
	ctl := NewContactWayGroupAPIController(context)

	uuidsInterface, _ := context.Get("uuids")
	uuids := uuidsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.contactWayGroup.delete")

	var err error
	err = globalDatabase.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		serviceContactWay := service.NewContactWayService(context)
		err = serviceContactWay.DetachContactWayGroup(tx, uuids)
		if err != nil {
			return err
		}
		err = ctl.ServiceContactWayGroup.DeleteContactWayGroups(globalDatabase.G_DBConnection, uuids)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_DELETE_CONTACT_WAY_GROUP, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}
