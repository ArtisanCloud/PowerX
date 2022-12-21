package root

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/service/wx"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type SystemAPIController struct {
	*api.APIController
}

func NewSystemAPIController(context *gin.Context) (ctl *SystemAPIController) {

	return &SystemAPIController{
		APIController: api.NewAPIController(context),
	}
}

func APIPing(context *gin.Context) {
	ctl := NewSystemAPIController(context)

	defer api.RecoverResponse(context, "api.root.system.ping")

	// 正常返回json
	ctl.RS.Success(context, "accepted")
	return
}

func APIPostDetect(context *gin.Context) {
	ctl := NewSystemAPIController(context)

	defer api.RecoverResponse(context, "api.root.system.post.detect")

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
	ctl := NewSystemAPIController(context)

	defer api.RecoverResponse(context, "api.root.system.get.detect")

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

func APIWXConfig(context *gin.Context) {
	ctl := NewSystemAPIController(context)

	defer api.RecoverResponse(context, "api.root.system.wx.config")

	serviceWX := wx.NewWXService(context)

	wxConfig := serviceWX.GetWXConfig()

	ctl.RS.Success(context, wxConfig)
	return

}
