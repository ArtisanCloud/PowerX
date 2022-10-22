package root

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerSocialite/v2/src/providers"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	root "github.com/ArtisanCloud/PowerX/app/http/request/root/install"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/weCom"
	globalConfig "github.com/ArtisanCloud/PowerX/config"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/ArtisanCloud/PowerX/routes/global"
	"github.com/gin-gonic/gin"
	"net/http"
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

func APIRootCheckInitialization(context *gin.Context) {
	ctl := NewInstallAPIController(context)

	defer api.RecoverResponse(context, "api.root.system.root.init.check")

	rootEmployee, err := ctl.ServiceInstall.CheckRootInitialization(context)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_CHECK_ROOT, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, rootEmployee)
}

func APIInitRoot(context *gin.Context) {

	ctl := NewInstallAPIController(context)
	// get user info from code
	root, err := weCom.G_WeComEmployee.AuthorizedEmployee(context)

	if err != nil {
		ctl.RS.SetCode(http.StatusExpectationFailed, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		ctl.RS.ThrowJSONResponse(context)
		return
	}

	strToken, rsCode := WeComGetRootToken(context, root)
	if rsCode != globalConfig.API_RESULT_CODE_INIT {
		ctl.RS.SetCode(rsCode, globalConfig.API_RETURN_CODE_ERROR, "", "")
		ctl.RS.ThrowJSONResponse(context)
		return
	}
	res := map[string]interface{}{
		"token_type":    "Bearer",
		"expires_in":    service.InExpiredSecond,
		"access_token":  strToken,
		"refresh_token": "",
	}

	// 正常返回json
	ctl.RS.Success(context, res)

}

func WeComGetRootToken(context *gin.Context, user *providers.User) (strToken string, rsCode int) {
	var root *models.Employee
	userID := user.GetID()
	if userID == "" {
		return "", globalConfig.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL
	}

	serviceEmployee := service.NewEmployeeService(context)
	root, _ = serviceEmployee.GetEmployeeByUserIDOnWXPlatform(context, userID)

	// query user detail by user id
	if user.GetOpenID() == "" {
		responseOpenID, err := weCom.G_WeComEmployee.App.User.UserIdToOpenID(userID)
		if err != nil || responseOpenID.OpenID == "" {
			return "", globalConfig.API_ERR_CODE_LACK_OF_WX_OPEN_ID
		}
		root.WXEmployee.WXCorpID = object.NewNullString(weCom.G_WeComEmployee.App.Config.GetString("corp_id", ""), true)
		root.WXEmployee.WXOpenID = object.NewNullString(responseOpenID.OpenID, true)
	}
	serviceWeComEmployee := weCom.NewWeComEmployeeService(nil)

	roleID, err := serviceEmployee.GetRootRoleID(globalDatabase.G_DBConnection)
	if err != nil {
		return "", globalConfig.API_ERR_CODE_FAIL_TO_GET_ROLE_SUPER_ADMIN_ID
	}
	root.RoleID = &roleID
	err = serviceWeComEmployee.UpsertEmployeeByWXEmployee(globalDatabase.G_DBConnection, root)
	if err != nil {
		return "", globalConfig.API_ERR_CODE_FAIL_TO_UPSERT_EMPLOYEE
	}

	serviceAuth := service.NewAuthService(context)
	strToken, _ = serviceAuth.CreateTokenForEmployee(root)

	return strToken, globalConfig.API_RESULT_CODE_INIT
}
