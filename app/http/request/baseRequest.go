package request

import (
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/configs/global"
	"github.com/gin-gonic/gin"
)

func ValidatePara(context *gin.Context, form interface{}) (err error) {

	if err = context.ShouldBind(form); err != nil {
		if err = context.ShouldBindJSON(form); err != nil {
			apiResponse := http.NewAPIResponse(context)
			apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
			return err
		}
	}
	return err

}
