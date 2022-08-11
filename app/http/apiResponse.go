package http

import (
	fmt2 "fmt"
	"github.com/ArtisanCloud/PowerX/app/service"
	. "github.com/ArtisanCloud/PowerX/config"
	. "github.com/ArtisanCloud/PowerX/config/global"
	"github.com/gin-gonic/gin"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"net/http"
)

type Meta struct {
	ResultCode    int    `json:"result_code"`
	ResultMessage string `json:"result_message"`
	ReturnCode    int    `json:"return_code"`
	ReturnMessage string `json:"return_message"`
	Locale        string `json:"locale"`
	Timezone      string `json:"timezone"`
}

type APIResponse struct {
	Context *gin.Context `json:"-"`

	Meta Meta        `json:"meta"`
	Data interface{} `json:"data"`

	Printer *message.Printer
	locale  string
}

func NewAPIResponse(ctx *gin.Context) (rs *APIResponse) {
	if ctx == nil {
		ctx = &gin.Context{}
	}

	ctx.Header("version", APP_VERSION)

	// set locale
	local := service.GetSessionLocale(ctx)
	var p *message.Printer
	if local == service.LOCALE_EN {
		p = message.NewPrinter(language.English)
	} else {
		p = message.NewPrinter(language.Chinese)
	}

	rs = &APIResponse{
		Context: ctx,
		Meta: Meta{
			ReturnCode:    API_RETURN_CODE_INIT,
			ReturnMessage: "",
			ResultCode:    API_RESULT_CODE_INIT,
			ResultMessage: "",
		},
		Data: nil,

		Printer: p,
		locale:  local,
	}
	return rs
}

func (rs *APIResponse) IsNoError() bool {
	return rs.Meta.ReturnCode == API_RETURN_CODE_INIT
}

/*----------- Return --------------*/
/** Get Return Code */
func (rs *APIResponse) GetReturnCode() int {
	return rs.Meta.ReturnCode
}

/** Set Return Code */
func (rs *APIResponse) SetReturnCode(code int, returnMSG string) *APIResponse {
	rs.Meta.ReturnCode = code
	rs.Meta.ReturnMessage = returnMSG

	return rs
}

/** Set Return Message */
func (rs *APIResponse) SetReturnMessage(message string) *APIResponse {
	rs.Meta.ReturnMessage = message
	return rs
}

/** Get Return Message */
func (rs *APIResponse) GetReturnMessage() string {

	return rs.Meta.ReturnMessage
}

/*----------- Result --------------*/
/** Get Result Code */
func (rs *APIResponse) GetResultCode() int {
	return rs.Meta.ResultCode
}

/** Set Result Code */
func (rs *APIResponse) SetResultCode(code int, resultMSG string) *APIResponse {
	rs.Meta.ResultCode = code
	rs.Meta.ResultMessage = resultMSG

	return rs
}

/** Get Result Message */
func (rs *APIResponse) GetResultMessage() string {
	return rs.Meta.ResultMessage
}

/** Set Result Message */
func (rs *APIResponse) SetResultMessage(message string) *APIResponse {
	rs.Meta.ResultMessage = message
	return rs
}

/*----------- Data --------------*/
/** Get Data */
func (rs *APIResponse) GetData() interface{} {
	return rs.Data
}

/** Set Data */
func (rs *APIResponse) SetData(data interface{}) *APIResponse {
	rs.Data = data
	return rs
}

/** Get Response Json */
func (rs *APIResponse) getJsonResponseBody() map[string]interface{} {

	var (
		rtMsg string
		rsMsg string
	)

	if rs.Meta.ReturnMessage != "" {
		rtMsg = rs.Meta.ReturnMessage
	}

	if rs.Meta.ResultMessage != "" {
		rsMsg = rs.Meta.ResultMessage
	}

	if rtMsg == "" {
		rtMsg = rs.Printer.Sprintf(fmt2.Sprintf("%d", rs.Meta.ReturnCode))
	}
	if rsMsg == "" {
		rsMsg = rs.Printer.Sprintf(fmt2.Sprintf("%d", rs.Meta.ResultCode))
	}
	//fmt2.Printf("local:%s %d %s, %d %s", local, rs.Meta.ReturnCode, rtMsg, rs.Meta.ResultCode, rsMsg)

	// return map
	return map[string]interface{}{
		"meta": gin.H{
			"return_code":    rs.Meta.ReturnCode,
			"return_message": rtMsg,
			"result_code":    rs.Meta.ResultCode,
			"result_message": rsMsg,
			"timezone":       AppConfigure.Timezone,
			"locale":         AppConfigure.Locale,
		},
		"data": rs.Data,
	}
}

func (rs *APIResponse) SetCode(resultCode int, returnCode int, returnMSG string, resultMSG string) *APIResponse {

	rs.SetReturnCode(returnCode, returnMSG)
	rs.SetResultCode(resultCode, resultMSG)
	return rs
}

/** Reset Codes */
func (rs *APIResponse) ResetCodes() *APIResponse {
	rs.ResetReturnCode()
	rs.ResetResultCode()
	return rs
}

/** Reset Codes */
func (rs *APIResponse) ResetReturnCode() {
	rs.Meta.ReturnCode = API_RETURN_CODE_INIT
}

/** Reset Codes */
func (rs *APIResponse) ResetResultCode() {
	rs.Meta.ResultCode = API_RESULT_CODE_INIT
}

/*----------- Response --------------*/
/** Success Json Response */
func (rs *APIResponse) Success(context *gin.Context, data interface{}) {
	rs.ResetCodes()

	if data != nil {
		rs.SetData(data)
	}

	body := rs.getJsonResponseBody()
	context.JSON(http.StatusOK, body)
}

/** Error Json Response */
func (rs *APIResponse) Error(context *gin.Context, resultCode int, resultMessage string, returnMessage string) {
	body := rs.getJsonResponseBody()
	context.AbortWithStatusJSON(http.StatusBadRequest, body)
}

/** Throw Json Response */
func (rs *APIResponse) ThrowJSONResponse(context *gin.Context) {
	body := rs.getJsonResponseBody()
	context.AbortWithStatusJSON(http.StatusBadRequest, body)

}
