package wx

import (
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/oauth/request"
	"github.com/gin-gonic/gin"
	"net/http"
)

func ValidateRequestOAuthCallback(context *gin.Context) {
	var form request.ParaOAuthCallback

	//logger.Info("validate make reservation", nil)
	if err := context.ShouldBind(&form); err != nil {
		context.AbortWithStatusJSON(http.StatusBadRequest, "Lack of parameters!")
	}

	context.Set("params", form)

	context.Next()
}

