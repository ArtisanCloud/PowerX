package admin

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	requestTag "github.com/ArtisanCloud/PowerX/app/http/request/admin/tag"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/configs/global"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type TagAPIController struct {
	*api.APIController
	ServiceTag *service.TagService
}

func NewTagAPIController(context *gin.Context) (ctl *TagAPIController) {

	return &TagAPIController{
		APIController: api.NewAPIController(context),
		ServiceTag:    service.NewTagService(context),
	}
}

func APIGetGroupChatTagGroupList(context *gin.Context) {
	ctl := NewTagAPIController(context)

	params, _ := context.Get("params")
	para := params.(request.ParaList)

	defer api.RecoverResponse(context, "api.admin.tag.group.list")

	conditions := &map[string]interface{}{
		"owner_type": (&models.GroupChat{}).GetTableName(true),
	}
	arrayList, err := ctl.ServiceTag.GetGroupList(globalDatabase.G_DBConnection, conditions, para.Page, para.PageSize)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_WX_TAG_LIST, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

// ---------------------------------------------------------------------------------------------------------------------

func APIGetTagGroupList(context *gin.Context) {
	ctl := NewTagAPIController(context)

	params, _ := context.Get("params")
	para := params.(*requestTag.ParaTagGroupList)

	defer api.RecoverResponse(context, "api.admin.tag.group.list")

	arrayList, err := ctl.ServiceTag.QueryTagList(globalDatabase.G_DBConnection, para.Type, para.GroupID)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_WX_TAG_LIST, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetTagGroupDetail(context *gin.Context) {
	ctl := NewTagAPIController(context)

	params, _ := context.Get("groupID")
	groupID := params.(string)

	defer api.RecoverResponse(context, "api.admin.tag.group.detail")

	account, err := ctl.ServiceTag.GetTagGroupByID(globalDatabase.G_DBConnection, groupID)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_WX_TAG_DETAIL, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, account)
}

func APIInsertTagGroup(context *gin.Context) {
	ctl := NewTagAPIController(context)

	tagGroupInterface, _ := context.Get("tagGroup")
	tagGroup := tagGroupInterface.(*tag.TagGroup)
	tagsInterface, _ := context.Get("tags")
	tags := tagsInterface.([]*tag.Tag)
	defer api.RecoverResponse(context, "api.admin.tag.insert")

	//fmt.Dump(tagGroup, tags)
	var err error

	// upsert wx tag group

	err = ctl.ServiceTag.CreateTagGroupWithTags(globalDatabase.G_DBConnection, tagGroup, tags)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_INSERT_WX_TAG_GROUP, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	tagGroup.Tags = tags

	ctl.RS.Success(context, tagGroup)

}

func APIUpdateTagGroup(context *gin.Context) {
	ctl := NewTagAPIController(context)

	tagGroupInterface, _ := context.Get("tagGroup")
	tagGroup := tagGroupInterface.(*tag.TagGroup)
	tagsInterface, _ := context.Get("tags")
	tags := tagsInterface.([]*tag.Tag)
	defer api.RecoverResponse(context, "api.admin.tag.update")

	//fmt.Dump(tagGroup, tags)
	var err error

	// upsert wx tag group

	err = ctl.ServiceTag.UpdateTagGroupWithTags(globalDatabase.G_DBConnection, tagGroup, tags)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_INSERT_WX_TAG_GROUP, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	tagGroup.Tags = tags

	ctl.RS.Success(context, tagGroup)
}

func APIDeleteTagGroups(context *gin.Context) {
	ctl := NewTagAPIController(context)

	groupIDsInterface, _ := context.Get("groupIDs")
	groupIDs := groupIDsInterface.([]string)
	tagIDsInterface, _ := context.Get("tagIDs")
	tagIDs := tagIDsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.tag.delete")

	var err error

	// delete wx tag group
	err = ctl.ServiceTag.DeleteTagGroupsWithTags(globalDatabase.G_DBConnection, groupIDs, tagIDs)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_DELETE_WX_TAG_GROUP, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}

func APIBindTagsToGroupChat(context *gin.Context) {

	ctl := NewTagAPIController(context)

	groupChatsInterface, _ := context.Get("groupChats")
	groupChats := groupChatsInterface.([]*models.GroupChat)
	tagsInterface, _ := context.Get("tags")
	tags := tagsInterface.([]*tag.Tag)

	defer api.RecoverResponse(context, "api.admin.customer.bind.tags")

	var err error
	for _, groupChat := range groupChats {
		err = ctl.ServiceTag.SyncTagsToObject(globalDatabase.G_DBConnection, groupChat, tags)
		if err != nil {
			ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_SYNC_TAG, global.API_RETURN_CODE_ERROR, "", err.Error())
			panic(ctl.RS)
			return
		}
	}

	ctl.RS.Success(context, err)

}
