package groupChat

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
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

	tagGroup, tags := convertParaToTagGroupForUpsert(&form)
	context.Set("tagGroup", tagGroup)
	context.Set("tags", tags)
	context.Next()
}

func convertParaToTagGroupForUpsert(form *ParaInsertTagGroup) (tagGroup *tag.TagGroup, tags []*tag.Tag) {

	ownerType := (&models.GroupChat{}).GetTableName(true)
	tagGroup = &tag.TagGroup{
		PowerCompactModel: database.NewPowerCompactModel(),
		GroupName:         form.GroupName,
		OwnerType:         ownerType,
	}
	tagGroup.UniqueID = tagGroup.GetComposedUniqueID()

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

	return tagGroup, tags
}
