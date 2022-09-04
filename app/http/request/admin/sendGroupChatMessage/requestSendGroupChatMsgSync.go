package sendGroupChatMsg

import (
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
)

type ParaSendGroupChatMsgSync struct {
	StartDatetime string `form:"startDatetime" json:"startDatetime"`
	EndDatetime   string `form:"endDatetime" json:"endDatetime"`
	Limit         int    `form:"limit" json:"limit"`
}

func ValidateSendGroupChatMsgSync(context *gin.Context) {
	var form ParaSendGroupChatMsgSync

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	startDatetime, endDatetime, limit, err := convertParaSendGroupChatMsgSync(form)
	apiResponse := http.NewAPIResponse(context)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).
			ThrowJSONResponse(context)
		return
	}

	context.Set("startDatetime", &startDatetime)
	context.Set("endDatetime", &endDatetime)
	context.Set("limit", limit)
	context.Next()
}

func convertParaSendGroupChatMsgSync(form ParaSendGroupChatMsgSync) (startDatetime carbon.Carbon, endDatetime carbon.Carbon, limit int, err error) {

	startDatetime = carbon.Parse(form.StartDatetime)
	endDatetime = carbon.Parse(form.EndDatetime)

	if startDatetime.IsZero() {
		now := carbon.Now()
		startDatetime = now.AddDays(-30)
		endDatetime = now
	}
	if form.Limit <= 0 {
		limit = 100
	}

	return startDatetime, endDatetime, limit, nil
}
