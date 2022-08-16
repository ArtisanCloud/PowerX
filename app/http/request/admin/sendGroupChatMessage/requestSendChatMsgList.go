package sendGroupChatMsg

import (
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/configs/global"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
)

type ParaSendGroupChatMsgList struct {
	request.ParaList

	GroupChatMsgName string   `form:"groupChatMsgName" json:"groupChatMsgName"`
	CreatorUserIDs   []string `form:"creatorUserIDs" json:"creatorUserIDs"`
	FilterStartDate  string   `form:"filterStartDate" json:"filterStartDate"`
	FilterEndDate    string   `form:"filterEndDate" json:"filterEndDate"`
}

func ValidateSendGroupChatMsgList(context *gin.Context) {
	var form ParaSendGroupChatMsgList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	groupChatMsgName, creatorUserIDs, filterStartDate, filterEndDate, err := convertParaSendGroupChatMsgList(form)
	apiResponse := http.NewAPIResponse(context)
	if err != nil {
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).
			ThrowJSONResponse(context)
		return
	}

	context.Set("params", form.ParaList)
	context.Set("groupChatMsgName", groupChatMsgName)
	context.Set("creatorUserIDs", creatorUserIDs)
	context.Set("filterStartDate", &filterStartDate)
	context.Set("filterEndDate", &filterEndDate)
	context.Next()
}

func convertParaSendGroupChatMsgList(form ParaSendGroupChatMsgList) (groupChatMsgName string, creatorUserIDs []string, filterStartDate carbon.Carbon, filterEndDate carbon.Carbon, err error) {

	// Get senders
	groupChatMsgName = form.GroupChatMsgName
	creatorUserIDs = form.CreatorUserIDs

	filterStartDate = carbon.Parse(form.FilterStartDate)
	filterEndDate = carbon.Parse(form.FilterEndDate)

	return groupChatMsgName, creatorUserIDs, filterStartDate, filterEndDate, nil
}
