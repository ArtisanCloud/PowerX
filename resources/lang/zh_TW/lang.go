package en_TW

import (
	fmt2 "fmt"
	"github.com/ArtisanCloud/PowerX/configs/global"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var lang = language.Chinese

func LoadLang() {
	message.SetString(lang, fmt2.Sprintf("%d", global.API_RETURN_CODE_INIT), "")
	message.SetString(lang, fmt2.Sprintf("%d", global.API_RETURN_CODE_WARNING), "警告訊息")
	message.SetString(lang, fmt2.Sprintf("%d", global.API_RETURN_CODE_ERROR), "錯誤訊息")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_RESULT_CODE_INIT), "")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_RESULT_CODE_SUCCESS_RESET_PASSWORD), "密碼修改成功")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_WARNING_CODE_IN_MAINTENANCE), "系統維護中")
	message.SetString(lang, fmt2.Sprintf("%d", global.API_WARNING_CODE_NEED_UPDATE), "推出新版本，更新APP後再重新操作。")

	message.SetString(lang, fmt2.Sprintf("%d", global.API_ERR_CODE_REQUEST_PARAM_ERROR), "請求參數錯誤")

}
