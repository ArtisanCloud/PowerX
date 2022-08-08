package wx

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/groupChat"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type GroupChatAPIController struct {
	*api.APIController
	ServiceGroupChat *service.GroupChatService
	ServiceWXTag     *wecom.WXTagService
}

func NewGroupChatAPIController(context *gin.Context) (ctl *GroupChatAPIController) {

	return &GroupChatAPIController{
		APIController:    api.NewAPIController(context),
		ServiceGroupChat: service.NewGroupChatService(context),
		ServiceWXTag:     wecom.NewWXTagService(context),
	}
}

func APIGroupChatSync(context *gin.Context) {
	ctl := NewGroupChatAPIController(context)

	defer api.RecoverResponse(context, "api.admin.groupChat.sync")

	// sync wx tag group from wx platform for a month
	err := ctl.ServiceGroupChat.SyncGroupChatFromWXPlatform(0, nil, "", 1000)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_SYNC_GROUP_CHAT_ON_WX_PLATFORM, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}

func APIGetGroupChatList(context *gin.Context) {
	ctl := NewGroupChatAPIController(context)

	params, _ := context.Get("params")
	para := params.(*groupChat.ParaGroupChatList)

	defer api.RecoverResponse(context, "api.admin.groupChat.list")

	arrayList, err := ctl.ServiceGroupChat.GetQueryList(global.DBConnection,
		para.AdminUserID, para.Name,
		para.TagIDs,
		para.SortBy, para.Ascend,
		para.StartDate, para.Status)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetGroupChatDetail(context *gin.Context) {
	ctl := NewGroupChatAPIController(context)

	chatIDInterface, _ := context.Get("chatID")
	chatID := chatIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.groupChat.detail")

	groupChat, err := ctl.ServiceGroupChat.GetGroupChatByChatID(global.DBConnection, chatID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	if groupChat == nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_DETAIL, config.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	groupChat.Tags, err = groupChat.LoadTags(global.DBConnection, nil)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	groupChat.WXGroupChatMembers, err = groupChat.LoadWXGroupChatMembers(global.DBConnection, nil)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	groupChat.WXGroupChatAdmins, err = groupChat.LoadWXGroupChatAdmins(global.DBConnection, nil)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, groupChat)
}

// ------------------------------------------------------------

func APIGetGroupChatListOnWXPlatform(context *gin.Context) {
	ctl := NewGroupChatAPIController(context)

	params, _ := context.Get("params")
	para := params.(*groupChat.ParaWXPlatformGroupChatList)

	defer api.RecoverResponse(context, "api.admin.groupChat.list")

	arrayList, err := ctl.ServiceGroupChat.GetGroupChatListOnWXPlatform(para.StatusFilter, para.OwnerFilter, para.Cursor, para.Limit)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetGroupChatDetailOnWXPlatform(context *gin.Context) {
	ctl := NewGroupChatAPIController(context)

	params, _ := context.Get("params")
	para := params.(*groupChat.ParaWXPlatformGroupChatDetail)

	responseGroupChat, err := wecom.WeComApp.App.ExternalContactGroupChat.Get(para.ChatID, para.NeedName)

	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, responseGroupChat)

}
