package tag

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXTagGroupDetail struct {
	GroupID string `form:"groupID" json:"groupID" binding:"required"`
}

func ValidateWXTagGroupDetail(context *gin.Context) {
	var form ParaWXTagGroupDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("groupID", form.GroupID)
	context.Next()
}
