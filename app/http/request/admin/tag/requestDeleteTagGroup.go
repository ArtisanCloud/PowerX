package tag

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaDeleteTagGroup struct {
	GroupIDs []string `form:"groupIDs" json:"groupIDs"`
	TagIDs   []string `form:"tagIDs" json:"tagIDs"`
}

func ValidateDeleteTagGroup(context *gin.Context) {
	var form ParaDeleteTagGroup

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("groupIDs", form.GroupIDs)
	context.Set("tagIDs", form.TagIDs)
	context.Next()
}
