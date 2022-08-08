package groupChat

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type ParaBindTagsToGroupChats struct {
	GroupChatIDs []string `form:"groupChatIDs" json:"groupChatIDs" binding:"required"`
	TagIDs       []string `form:"tagIDs" json:"tagIDs" binding:"required,min=1"`
}

func ValidateBindTagsToGroupChats(context *gin.Context) {
	var form ParaBindTagsToGroupChats

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)
	groupChat, wxTags, err := convertParaToBindTagsToGroupChat(&form)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
	}
	context.Set("groupChats", groupChat)
	context.Set("tags", wxTags)
	context.Next()
}

func convertParaToBindTagsToGroupChat(form *ParaBindTagsToGroupChats) (groupChats []*models.GroupChat, tags []*tag.Tag, err error) {

	serviceGroupChat := service.NewGroupChatService(nil)
	groupChats, err = serviceGroupChat.GetGroupChatsByChatIDs(global.DBConnection, form.GroupChatIDs)
	if err != nil {
		return groupChats, tags, err
	}

	serviceTag := service.NewTagService(nil)
	tags, err = serviceTag.GetTagsByIDs(global.DBConnection, form.TagIDs)

	return groupChats, tags, err
}
