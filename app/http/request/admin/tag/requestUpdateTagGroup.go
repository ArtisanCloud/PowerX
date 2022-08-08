package tag

import (
	"errors"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type ParaUpdateTagGroup struct {
	GroupID   string     `form:"groupID" json:"groupID" binding:"required"`
	GroupName string     `form:"groupName" json:"groupName" binding:"required"`
	Tags      []*tag.Tag `form:"tags" json:"tags" binding:"required,min=1"`
}

func ValidateUpdateTagGroup(context *gin.Context) {
	var form ParaUpdateTagGroup

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	apiResponse := http.NewAPIResponse(context)

	tagGroup, tags, err := convertParaToTagGroupForUpdate(&form)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}

	context.Set("tagGroup", tagGroup)
	context.Set("tags", tags)
	context.Next()
}

func convertParaToTagGroupForUpdate(form *ParaUpdateTagGroup) (tagGroup *tag.TagGroup, tags []*tag.Tag, err error) {

	serviceTag := service.NewTagService(nil)
	tagGroup, err = serviceTag.GetTagGroupByID(global.DBConnection, form.GroupID)
	if err != nil {
		return tagGroup, tags, err
	}
	if tagGroup == nil {
		return tagGroup, tags, errors.New("tag group not found")
	}

	tagGroup.GroupName = form.GroupName

	for _, paraTag := range form.Tags {
		updateTag := &tag.Tag{
			PowerCompactModel: databasePowerLib.NewPowerCompactModel(),
			Name:              paraTag.Name,
			GroupID:           tagGroup.UniqueID,
			Type:              paraTag.Type,
		}
		updateTag.UniqueID = updateTag.GetComposedUniqueID()
		tags = append(tags, updateTag)
	}

	return tagGroup, tags, err
}
