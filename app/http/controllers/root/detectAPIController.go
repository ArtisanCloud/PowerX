package root

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type DetectAPIController struct {
	*api.APIController
}

func NewDetectAPIController(context *gin.Context) (ctl *DetectAPIController) {

	return &DetectAPIController{
		APIController: api.NewAPIController(context),
	}
}

func APIPing(context *gin.Context) {
	ctl := NewDetectAPIController(context)

	// 正常返回json
	ctl.RS.Success(context, "accepted")
	return
}

func APIPostDetect(context *gin.Context) {
	ctl := NewDetectAPIController(context)

	postForm := &object.HashMap{}
	err := request.ValidatePara(context, postForm)
	if err != nil {
		ctl.RS.Error(context, config.API_ERR_CODE_REQUEST_PARAM_ERROR, err.Error(), "")
	}

	// 正常返回json
	ctl.RS.Success(context, postForm)
	return
}

func APIGetDetect(context *gin.Context) {
	ctl := NewDetectAPIController(context)

	values := context.Request.URL.Query()
	getQuery := object.StringMap{}
	for k, v := range values {
		getQuery[k] = v[0]
	}
	//if err != nil {
	//	ctl.RS.Error(context, config.API_ERR_CODE_REQUEST_PARAM_ERROR, err.Error(), "")
	//}

	// 正常返回json
	ctl.RS.Success(context, getQuery)
	return
}
