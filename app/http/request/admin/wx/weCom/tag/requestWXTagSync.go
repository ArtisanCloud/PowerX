package tag

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXTagGroupSync struct {
	GroupIDs []string `form:"groupIDs" json:"groupIDs"`
	TagIDs   []string `form:"tagIDs" json:"tagIDs"`
}

func ValidateWXTagGroupSync(context *gin.Context) {
	var form ParaWXTagGroupSync

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("groupIDs", form.GroupIDs)
	context.Set("tagIDs", form.TagIDs)
	context.Next()
}
