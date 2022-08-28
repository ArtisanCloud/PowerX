package sendChatMsg

import (
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/config/global"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
)

type ParaSendChatMsgList struct {
	request.ParaList

	CreatorUserIDs  []string `form:"creatorUserIDs" json:"creatorUserIDs"`
	FilterStartDate string   `form:"filterStartDate" json:"filterStartDate"`
	FilterEndDate   string   `form:"filterEndDate" json:"filterEndDate"`
}

func ValidateSendChatMsgList(context *gin.Context) {
	var form ParaSendChatMsgList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	creatorUserIDs, filterStartDate, filterEndDate, err := convertParaSendChatMsgList(form)
	apiResponse := http.NewAPIResponse(context)
	if err != nil {
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).
			ThrowJSONResponse(context)
		return
	}

	context.Set("params", form.ParaList)

	context.Set("creatorUserIDs", creatorUserIDs)
	context.Set("filterStartDate", &filterStartDate)
	context.Set("filterEndDate", &filterEndDate)
	context.Next()
}

func convertParaSendChatMsgList(form ParaSendChatMsgList) (creatorUserIDs []string, filterStartDate carbon.Carbon, filterEndDate carbon.Carbon, err error) {

	// Get senders
	creatorUserIDs = form.CreatorUserIDs

	filterStartDate = carbon.Parse(form.FilterStartDate)
	filterEndDate = carbon.Parse(form.FilterEndDate)

	return creatorUserIDs, filterStartDate, filterEndDate, nil
}
