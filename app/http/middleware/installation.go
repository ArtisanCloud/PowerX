package middleware

import (
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

func CheckInstalled(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	if !config.G_AppConfigure.SystemConfig.Installed {
		apiResponse.SetCode(config.API_WARNING_CODE_SYSTEM_NOT_INSTALLED, config.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

}

func CheckNotInstalled(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	if config.G_AppConfigure.SystemConfig.Installed {
		apiResponse.SetCode(config.API_WARNING_CODE_SYSTEM_INSTALLED, config.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

}
