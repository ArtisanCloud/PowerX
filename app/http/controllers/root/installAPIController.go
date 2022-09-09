package root

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	root "github.com/ArtisanCloud/PowerX/app/http/request/root/install"
	"github.com/ArtisanCloud/PowerX/app/service"
	globalConfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/routes/global"
	"github.com/gin-gonic/gin"
)

type InstallAPIController struct {
	*api.APIController
	ServiceInstall *service.InstallService
}

func NewInstallAPIController(context *gin.Context) (ctl *InstallAPIController) {

	return &InstallAPIController{
		APIController:  api.NewAPIController(context),
		ServiceInstall: service.NewInstallService(context),
	}
}

func APISystemShutDown(context *gin.Context) {
	ctl := NewInstallAPIController(context)

	defer api.RecoverResponse(context, "api.root.system.shutDown")
	err := global.G_Server.Shutdown(context)

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_SHUT_DOWN_SYSTEM, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)

}
func APISystemInstall(context *gin.Context) {
	ctl := NewInstallAPIController(context)

	params, _ := context.Get("params")
	para := params.(*root.ParaSystemInstall)

	defer api.RecoverResponse(context, "api.root.system.install")

	arrayList, err := ctl.ServiceInstall.InstallSystem(para.AppConfig)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_INSTALL_SYSTEM, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APISystemCheckInstallation(context *gin.Context) {
	ctl := NewInstallAPIController(context)

	defer api.RecoverResponse(context, "api.root.system.install.check")

	arrayList, err := ctl.ServiceInstall.CheckSystemInstallation()
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_INSTALL_SYSTEM, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}
