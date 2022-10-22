package root

import (
	request2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/oauth/request"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

func ValidateInitRoot(context *gin.Context) {
	var form request2.ParaOAuthCallback

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)

	// 检查是否系统已经有root被初始化过
	serviceInstall := service.NewInstallService(context)
	root, err := serviceInstall.CheckRootInitialization(context)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}

	if root != nil {
		apiResponse.SetCode(config.API_ERR_CODE_ROOT_HAS_BEEN_INITIALIZED, config.API_RETURN_CODE_ERROR, "", "").ThrowJSONResponse(context)
		return
	}

	context.Set("params", form)
	context.Next()
}
