package tag

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/config/global"
	"github.com/gin-gonic/gin"
)

type ParaInsertTagGroup struct {
	GroupName string     `form:"groupName" json:"groupName"`
	Tags      []*tag.Tag `form:"tags" json:"tags" binding:"required,min=1"`
}

func ValidateInsertTagGroup(context *gin.Context) {
	var form ParaInsertTagGroup

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	tagGroup, tags, err := convertParaToTagGroupForUpsert(&form)
	if err != nil {
		apiResponse := http.NewAPIResponse(context)
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}
	context.Set("tagGroup", tagGroup)
	context.Set("tags", tags)
	context.Next()
}

func convertParaToTagGroupForUpsert(form *ParaInsertTagGroup) (tagGroup *tag.TagGroup, tags []*tag.Tag, err error) {

	if form.GroupName == "" {
		// tags to create without group
		tagGroup = &tag.TagGroup{
			UniqueID: "",
		}

	} else {
		tagGroup = &tag.TagGroup{
			PowerCompactModel: database.NewPowerCompactModel(),
			GroupName:         form.GroupName,
			OwnerType:         tag.DEFAULT_OWNER_TYPE,
		}
		tagGroup.UniqueID = tagGroup.GetComposedUniqueID()
	}

	for _, paraTag := range form.Tags {
		tag := &tag.Tag{
			PowerCompactModel: database.NewPowerCompactModel(),
			Name:              paraTag.Name,
			GroupID:           tagGroup.UniqueID,
			Type:              paraTag.Type,
		}
		tag.UniqueID = tag.GetComposedUniqueID()
		tags = append(tags, tag)
	}

	return tagGroup, tags, err
}
