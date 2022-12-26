package contactWay

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaContactWayList struct {
	GroupUUID string `form:"groupUUID" json:"groupUUID" xml:"groupUUID"`
	Name      string `form:"name" json:"name" xml:"name"`
	UserID    string `form:"userID" json:"userID" xml:"userID"`
}

func ValidateContactWayList(context *gin.Context) {
	var form ParaContactWayList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
