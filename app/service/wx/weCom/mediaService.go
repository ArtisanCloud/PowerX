package weCom

import (
	"github.com/ArtisanCloud/PowerLibs/v2/http/contract"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/power"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/media/response"
	"github.com/gin-gonic/gin"
)

type WeComMediaService struct {
	*WeComService
}

func NewWeComMediaService(ctx *gin.Context) (r *WeComMediaService) {
	r = &WeComMediaService{
		WeComService: G_WeComApp,
	}
	return r
}

func (srv *WeComMediaService) UploadImage(path string, data *power.HashMap) (*response.ResponseUploadImage, error) {
	return G_WeComApp.App.Media.UploadImage(path, data)
}

func (srv *WeComMediaService) UploadTempImage(path string, data *power.HashMap) (*response.ResponseUploadMedia, error) {
	return G_WeComApp.App.Media.UploadTempImage(path, data)
}

func (srv *WeComMediaService) UploadTempVoice(path string, data *power.HashMap) (*response.ResponseUploadMedia, error) {
	return G_WeComApp.App.Media.UploadTempVoice(path, data)
}

func (srv *WeComMediaService) UploadTempVideo(path string, data *power.HashMap) (*response.ResponseUploadMedia, error) {
	return G_WeComApp.App.Media.UploadTempVideo(path, data)
}

func (srv *WeComMediaService) UploadTempFile(path string, data *power.HashMap) (*response.ResponseUploadMedia, error) {
	return G_WeComApp.App.Media.UploadTempFile(path, data)
}

func (srv *WeComMediaService) GetMedia(mediaID string) (contract.ResponseInterface, error) {
	return G_WeComApp.App.Media.Get(mediaID)

}

func (srv *WeComMediaService) GetJSSDK(mediaID string) (contract.ResponseInterface, error) {
	return G_WeComApp.App.Media.GetJSSDK(mediaID)
}
