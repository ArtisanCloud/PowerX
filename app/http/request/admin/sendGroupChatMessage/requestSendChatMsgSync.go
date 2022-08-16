package sendGroupChatMsg

import (
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/configs/global"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
)

type ParaSendChatMsgSync struct {
	StartDatetime string `form:"startDatetime" json:"startDatetime"`
	EndDatetime   string `form:"endDatetime" json:"endDatetime"`
	Limit         int    `form:"limit" json:"limit"`
}

func ValidateSendChatMsgSync(context *gin.Context) {
	var form ParaSendChatMsgSync

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	startDatetime, endDatetime, limit, err := convertParaSendChatMsgSync(form)
	apiResponse := http.NewAPIResponse(context)
	if err != nil {
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).
			ThrowJSONResponse(context)
		return
	}

	context.Set("startDatetime", &startDatetime)
	context.Set("endDatetime", &endDatetime)
	context.Set("limit", limit)
	context.Next()
}

func convertParaSendChatMsgSync(form ParaSendChatMsgSync) (startDatetime carbon.Carbon, endDatetime carbon.Carbon, limit int, err error) {

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
