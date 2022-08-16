package wx

import (
	os2 "github.com/ArtisanCloud/PowerLibs/v2/os"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/configs/global"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type WXTagGroupAPIController struct {
	*api.APIController
	ServiceWXTagGroup *wecom.WXTagGroupService
	ServiceWXTag      *wecom.WXTagService
}

func NewWXTagGroupAPIController(context *gin.Context) (ctl *WXTagGroupAPIController) {

	return &WXTagGroupAPIController{
		APIController:     api.NewAPIController(context),
		ServiceWXTagGroup: wecom.NewWXTagGroupService(context),
		ServiceWXTag:      wecom.NewWXTagService(context),
	}
}

func APIGetWXTagGroupSync(context *gin.Context) {
	ctl := NewWXTagGroupAPIController(context)

	groupIDsInterface, _ := context.Get("groupIDs")
	groupIDs := groupIDsInterface.([]string)
	tagIDsInterface, _ := context.Get("tagIDs")
	tagIDs := tagIDsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.wxTagGroup.sync")

	// sync wx tag group from wx platform
	err := ctl.ServiceWXTagGroup.SyncWXTagGroupsFromWXPlatform(tagIDs, groupIDs, true)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_SYNC_WX_TAG_GROUP_ON_WX_PLATFORM, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}

func APIGetWXTagGroupList(context *gin.Context) {
	ctl := NewWXTagGroupAPIController(context)

	params, _ := context.Get("wxDepartmentID")
	wxDepartmentID := params.(int)

	defer api.RecoverResponse(context, "api.admin.wxTagGroup.list")

	arrayList, err := ctl.ServiceWXTagGroup.GetList(globalDatabase.G_DBConnection, wxDepartmentID)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_WX_TAG_GROUP_LIST, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetWXTagGroupDetail(context *gin.Context) {
	ctl := NewWXTagGroupAPIController(context)

	params, _ := context.Get("wxGroupID")
	wxGroupID := params.(string)

	defer api.RecoverResponse(context, "api.admin.wxTagGroup.detail")

	wxTagGroup, err := ctl.ServiceWXTagGroup.GetWXTagGroup(globalDatabase.G_DBConnection, wxGroupID)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_WX_TAG_GROUP_DETAIL, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, wxTagGroup)
}

func APIInsertWXTagGroup(context *gin.Context) {
	ctl := NewWXTagGroupAPIController(context)

	params, _ := context.Get("wxTagGroup")
	wxTagGroup := params.(*modelWX.WXTagGroup)

	defer api.RecoverResponse(context, "api.admin.wxTagGroup.insert")

	var err error

	// get wecome agent id
	agentIDENV, err := os2.GetEnvInt("wecom_agent_id")
	agentID := int64(agentIDENV)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_WECOM_AGENT_ID_INVALID, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	// upload wx tag group
	result, err := ctl.ServiceWXTagGroup.CreateWXTagGroupOnWXPlatform(wxTagGroup, &agentID)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_INSERT_WX_TAG_GROUP_ON_WX_PLATFORM, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	// convert wx tag group response to wx tag group foundation
	wxTagGroup, err = ctl.ServiceWXTagGroup.ConvertResponseToWXTagGroup(result, wxTagGroup.WXDepartmentID)

	// upsert wx tag group
	err = ctl.ServiceWXTagGroup.UpsertWXTagGroups(globalDatabase.G_DBConnection, []*modelWX.WXTagGroup{wxTagGroup}, nil)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_INSERT_WX_TAG_GROUP, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, wxTagGroup)

}

func APIUpdateWXTagGroup(context *gin.Context) {
	ctl := NewWXTagGroupAPIController(context)

	params, _ := context.Get("wxTagGroup")
	wxTagGroup := params.(*modelWX.WXTagGroup)

	defer api.RecoverResponse(context, "api.admin.wxTagGroup.update")
	var err error

	// update wx tag group on wx platform
	err = ctl.ServiceWXTagGroup.UpdateWXTagGroupOnWXPlatform(wxTagGroup, nil)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_UPDATE_WX_TAG_GROUP_ON_WX_PLATFORM, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	// update wx tag group
	err = ctl.ServiceWXTagGroup.UpsertWXTagGroups(globalDatabase.G_DBConnection, []*modelWX.WXTagGroup{wxTagGroup}, []string{
		"group_name",
		"order",
	})
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_UPDATE_WX_TAG_GROUP, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}

func APIDeleteWXTagGroups(context *gin.Context) {
	ctl := NewWXTagGroupAPIController(context)

	groupIDsInterface, _ := context.Get("groupIDs")
	tagIDsInterface, _ := context.Get("tagIDs")
	groupIDs := groupIDsInterface.([]string)
	tagIDs := tagIDsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.wxTagGroup.delete")

	var err error

	// delete wx tag group on wx platform
	err = ctl.ServiceWXTagGroup.DeleteWXTagGroupOnWXPlatform(groupIDs, tagIDs, nil)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_DELETE_WX_TAG_GROUP_ON_WX_PLATFORM, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	// delete wx tag group
	err = ctl.ServiceWXTagGroup.DeleteWXTagGroups(groupIDs, tagIDs)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_DELETE_WX_TAG_GROUP, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}
