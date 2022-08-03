package contactWay

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/gin-gonic/gin"
)

type ParaUpsertContactWayGroup struct {
	UUID string `form:"uuid" json:"uuid" xml:"uuid"`

	GroupName string `form:"groupName" json:"groupName" binding:"required"`
}

func ValidateUpsertContactWayGroup(context *gin.Context) {
	var form ParaUpsertContactWayGroup

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	contactWayGroup := convertParaToContactWayGroupForUpsert(form)
	context.Set("contactWayGroup", contactWayGroup)
	context.Next()
}

func convertParaToContactWayGroupForUpsert(form ParaUpsertContactWayGroup) (contactWayGroup *models.ContactWayGroup) {

	var uuid string = ""
	if form.UUID != "" {
		uuid = form.UUID
	}
	//fmt.Dump(form)

	contactWayGroup = &models.ContactWayGroup{
		PowerModel: &database.PowerModel{
			UUID: uuid,
		},

		Name: form.GroupName,
	}

	return contactWayGroup
}
