package wx

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type WXTagAPIController struct {
	*api.APIController
	ServiceWXTag *wecom.WXTagService
}

func NewWXTagAPIController(context *gin.Context) (ctl *WXTagAPIController) {

	return &WXTagAPIController{
		APIController: api.NewAPIController(context),
		ServiceWXTag:  wecom.NewWXTagService(context),
	}
}

func APIGetWXTagList(context *gin.Context) {
	ctl := NewWXTagAPIController(context)

	params, _ := context.Get("params")
	para := params.(request.ParaList)

	defer api.RecoverResponse(context, "api.admin.customer.list")

	arrayList, err := ctl.ServiceWXTag.GetList(global.DBConnection, nil, para.Page, para.PageSize)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_WX_TAG_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetWXTagDetail(context *gin.Context) {
	ctl := NewWXTagAPIController(context)

	params, _ := context.Get("params")
	para := params.(request.ParaDetail)

	defer api.RecoverResponse(context, "api.admin.customer.detail")

	account, err := ctl.ServiceWXTag.GetWXTag(global.DBConnection, para.UUID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_WX_TAG_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, account)
}

func APIBindWXTagsToCustomerToEmployeeByContactWayTags(context *gin.Context) {

	ctl := NewWXTagAPIController(context)

	pivotInterface, _ := context.Get("pivot")
	pivot := pivotInterface.(*models.RCustomerToEmployee)

	contactWayInterface, _ := context.Get("contactWay")
	contactWay := contactWayInterface.(*models.ContactWay)

	err := ctl.ServiceWXTag.SyncWXTagsToObject(global.DBConnection, pivot, contactWay.WXTags)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_SYNC_WX_TAG, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}

func APIBindWXTagsToCustomerToEmployee(context *gin.Context) {

	ctl := NewWXTagAPIController(context)

	pivotInterface, _ := context.Get("pivot")
	pivot := pivotInterface.(*models.RCustomerToEmployee)

	tagsInterface, _ := context.Get("tags")
	tags := tagsInterface.([]*wx.WXTag)

	defer api.RecoverResponse(context, "api.admin.customer.bind.tags")

	//err := ctl.ServiceWXTag.AppendWXTagsToPivotCustomerToEmployee(global.DBConnection, customer, tags)
	err := ctl.ServiceWXTag.SyncWXTagsToObject(global.DBConnection, pivot, tags)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_SYNC_WX_TAG, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)

}
