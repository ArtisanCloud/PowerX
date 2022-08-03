package tag

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaDeleteWXTagGroup struct {
	GroupIDs []string `form:"groupIDs" json:"groupIDs"`
	TagIDs   []string `form:"tagIDs" json:"tagIDs"`
}

func ValidateDeleteWXTagGroup(context *gin.Context) {
	var form ParaDeleteWXTagGroup

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("groupIDs", form.GroupIDs)
	context.Set("tagIDs", form.TagIDs)
	context.Next()
}
