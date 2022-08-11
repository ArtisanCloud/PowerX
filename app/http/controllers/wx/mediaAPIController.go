package wx

import (
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/power"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/config/global"
	"github.com/gin-gonic/gin"
	"io"
	"os"
)

type WeComMediaAPIController struct {
	*api.APIController
	ServiceWeComMedia *wecom.WeComMediaService
}

func NewWeComMediaAPIController(context *gin.Context) (ctl *WeComMediaAPIController) {

	return &WeComMediaAPIController{
		APIController:     api.NewAPIController(context),
		ServiceWeComMedia: wecom.NewWeComMediaService(context),
	}
}

func APIWeComMediaUploadImage(context *gin.Context) {
	ctl := NewWeComMediaAPIController(context)

	pathInterface, _ := context.Get("path")
	path := pathInterface.(string)
	dataInterface, _ := context.Get("data")
	data := dataInterface.(*power.HashMap)

	defer api.RecoverResponse(context, "api.admin.wecomMedia.list")

	arrayList, err := ctl.ServiceWeComMedia.UploadImage(path, data)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_UPLOAD_IMAGE, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	_ = os.Remove(path)

	ctl.RS.Success(context, arrayList)
}

func APIWeComMediaGetMedia(context *gin.Context) {
	ctl := NewWeComMediaAPIController(context)

	mediaIDInterface, _ := context.Get("mediaID")
	mediaID := mediaIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.wecomMedia.list")

	result, err := ctl.ServiceWeComMedia.GetMedia(mediaID)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_GET_MEDIA, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	io.Copy(context.Writer, result.GetBody())

}

func APIWeComMediaUploadTempImage(context *gin.Context) {
	ctl := NewWeComMediaAPIController(context)

	pathInterface, _ := context.Get("path")
	path := pathInterface.(string)
	dataInterface, _ := context.Get("data")
	data := dataInterface.(*power.HashMap)

	defer api.RecoverResponse(context, "api.admin.wecomMedia.uppload.tempImage")

	result, err := ctl.ServiceWeComMedia.UploadTempImage(path, data)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_UPLOAD_MEDIA, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	_ = os.Remove(path)

	ctl.RS.Success(context, result)
}

func APIWeComMediaUploadTempVoice(context *gin.Context) {
	ctl := NewWeComMediaAPIController(context)

	pathInterface, _ := context.Get("path")
	path := pathInterface.(string)
	dataInterface, _ := context.Get("data")
	data := dataInterface.(*power.HashMap)

	defer api.RecoverResponse(context, "api.admin.wecomMedia.uppload.tempVoice")

	result, err := ctl.ServiceWeComMedia.UploadTempVoice(path, data)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_UPLOAD_MEDIA, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	_ = os.Remove(path)

	ctl.RS.Success(context, result)
}

func APIWeComMediaUploadTempVideo(context *gin.Context) {
	ctl := NewWeComMediaAPIController(context)

	pathInterface, _ := context.Get("path")
	path := pathInterface.(string)
	dataInterface, _ := context.Get("data")
	data := dataInterface.(*power.HashMap)

	defer api.RecoverResponse(context, "api.admin.wecomMedia.uppload.tempVideo")

	result, err := ctl.ServiceWeComMedia.UploadTempVideo(path, data)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_UPLOAD_MEDIA, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	_ = os.Remove(path)

	ctl.RS.Success(context, result)
}

func APIWeComMediaUploadTempFile(context *gin.Context) {
	ctl := NewWeComMediaAPIController(context)

	pathInterface, _ := context.Get("path")
	path := pathInterface.(string)
	dataInterface, _ := context.Get("data")
	data := dataInterface.(*power.HashMap)

	defer api.RecoverResponse(context, "api.admin.wecomMedia.uppload.tempFile")

	result, err := ctl.ServiceWeComMedia.UploadTempFile(path, data)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_UPLOAD_MEDIA, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	_ = os.Remove(path)

	ctl.RS.Success(context, result)
}
