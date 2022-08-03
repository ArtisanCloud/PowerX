package tag

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaTagGroupList struct {
	GroupID *string `form:"groupID" json:"groupID" binding:"required"`
	Type    *int    `form:"type" json:"type" binding:"required"`
}

func ValidateTagGroupList(context *gin.Context) {
	var form ParaTagGroupList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("params", &form)
	context.Next()
}
