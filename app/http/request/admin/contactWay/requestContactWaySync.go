package contactWay

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
)

type ParaContactWaySync struct {
	StartDatetime string `form:"startDatetime" json:"startDatetime" xml:"startDatetime"`
	EndDatetime   string `form:"endDatetime" json:"endDatetime" xml:"endDatetime"`
	Limit         int    `form:"limit" json:"limit" xml:"limit"`
}

func ValidateContactWaySync(context *gin.Context) {
	var form ParaContactWaySync

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	startDatetime := carbon.Parse(form.StartDatetime)
	if startDatetime.IsZero() {
		// sync wx tag group from wx platform for a month
		startDatetime = carbon.Now().AddDays(-30)
	}
	endDatetime := carbon.Parse(form.EndDatetime)
	if endDatetime.IsZero() {
		endDatetime = carbon.Now()
	}
	if form.Limit <= 0 {
		form.Limit = 1000
	}

	context.Set("startDatetime", &form.StartDatetime)
	context.Set("endDatetime", &form.EndDatetime)
	context.Set("limit", &form.Limit)
	context.Next()
}
