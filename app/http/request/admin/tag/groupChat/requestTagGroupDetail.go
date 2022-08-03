package groupChat

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaTagGroupDetail struct {
	GroupID string `form:"groupID" json:"groupID" binding:"required"`
}

func ValidateTagGroupDetail(context *gin.Context) {
	var form ParaTagGroupDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("groupID", form.GroupID)
	context.Next()
}
