package en_US

import (
	fmt2 "fmt"
	"github.com/ArtisanCloud/PowerX/configs/global"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var lang = language.English

func LoadLang() {

	message.SetString(lang, fmt2.Sprintf("%d", global.API_RETURN_CODE_INIT), "")
	message.SetString(lang, fmt2.Sprintf("%d", global.API_RETURN_CODE_WARNING), "return warning")
	message.SetString(lang, fmt2.Sprintf("%d", global.API_RETURN_CODE_ERROR), "return error")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_RESULT_CODE_INIT), "")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_RESULT_CODE_SUCCESS_RESET_PASSWORD), "Success")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_WARNING_CODE_IN_MAINTENANCE), "System in maintenance")
	message.SetString(lang, fmt2.Sprintf("%d", global.API_WARNING_CODE_NEED_UPDATE), "New version found, please update the MiniProgram")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_ERR_CODE_REQUEST_PARAM_ERROR), "invalid request parameter")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_ERR_CODE_FAIL_TO_GET_REPORT_LIST), "Failed to get the customer report list")
	message.SetString(lang, fmt2.Sprintf("%d", global.API_ERR_CODE_FAIL_TO_UPSERT_REPORT), "Failed to upsert the customer report")
	message.SetString(lang, fmt2.Sprintf("%d", global.API_ERR_CODE_FAIL_TO_DELETE_REPORT), "Failed to delete the customer report")

}
