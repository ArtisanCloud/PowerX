package contactWay

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
)

type ParaWXPlatformContactWayList struct {
	StartDatetime string `form:"startDatetime" json:"startDatetime" xml:"startDatetime"`
	EndDatetime   string `form:"endDatetime" json:"endDatetime" xml:"endDatetime"`
	Limit         int    `form:"limit" json:"limit" xml:"limit"`
}

func ValidateWXPlatformContactWayList(context *gin.Context) {
	var form ParaWXPlatformContactWayList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	startDatetime := carbon.Parse(form.StartDatetime)
	endDatetime := carbon.Parse(form.EndDatetime)
	context.Set("startDatetime", &startDatetime)
	context.Set("endDatetime", &endDatetime)
	context.Set("limit", form.Limit)
	context.Next()
}
