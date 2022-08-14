package wx

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	requestCustomer "github.com/ArtisanCloud/PowerX/app/http/request/api/customer"
	requestWX "github.com/ArtisanCloud/PowerX/app/http/request/wx"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/miniProgram"
	"github.com/ArtisanCloud/PowerX/config/global"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"net/http"
)

type MiniProgramAPIController struct {
	*api.APIController
	ServiceMiniProgram *miniProgram.MiniProgramService
}

func NewMiniProgramAPIController(context *gin.Context) (ctl *MiniProgramAPIController) {

	return &MiniProgramAPIController{
		APIController:      api.NewAPIController(context),
		ServiceMiniProgram: miniProgram.MiniProgramApp,
	}
}

func APIMiniProgramCode2Session(context *gin.Context) {
	ctl := NewMiniProgramAPIController(context)

	codeInterface, _ := context.Get("para")
	code := codeInterface.(*requestWX.ParaMiniProgramCode2Session)
	rs, err := ctl.ServiceMiniProgram.App.Auth.Session(code.Code)
	if err != nil {
		ctl.RS.SetCode(http.StatusExpectationFailed, global.API_RETURN_CODE_ERROR, "", err.Error())
		ctl.RS.ThrowJSONResponse(context)
		return
	}
	if rs.OpenID == "" {
		ctl.RS.SetCode(http.StatusExpectationFailed, global.API_RETURN_CODE_ERROR, "", rs.ErrMSG)
		ctl.RS.ThrowJSONResponse(context)
		return
	}

	serviceCustomer := service.NewCustomerService(context)
	customer, err := serviceCustomer.GetCustomerByOpenID(globalDatabase.G_DBConnection, rs.OpenID)
	appID := ctl.ServiceMiniProgram.App.GetConfig().GetString("app_id", "")
	if err != nil || customer == nil {
		customer = models.NewCustomer(object.NewCollection(&object.HashMap{
			"openID":     rs.OpenID,
			"appID":      appID,
			"sessionKey": rs.SessionKey,
		}))

	} else {
		customer.AppID = object.NewNullString(appID, true)
		customer.SessionKey = rs.SessionKey
	}
	err = serviceCustomer.UpsertCustomers(globalDatabase.G_DBConnection, []*models.Customer{customer}, nil)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_UPSERT_ACCOUNT, global.API_RETURN_CODE_ERROR, "", "failed to save employee")
		panic(ctl.RS)
		return
	}
	serviceAuth := service.NewAuthService(context)
	strToken, _ := serviceAuth.CreateTokenForCustomer(customer)

	res := map[string]interface{}{
		"token_type":    "Bearer",
		"expires_in":    service.InExpiredSecond,
		"access_token":  strToken,
		"refresh_token": "",
	}

	// 正常返回json
	ctl.RS.Success(context, res)

}

func APIUpdateCustomer(context *gin.Context) {
	ctl := NewMiniProgramAPIController(context)

	params, _ := context.Get("params")
	para := params.(*requestCustomer.ParaUpsertCustomer)

	customer := service.GetAuthCustomer(context)

	defer api.RecoverResponse(context, "api.customer.upsert")
	var err error
	// ----

	userData, _ := ctl.ServiceMiniProgram.App.Encryptor.DecryptData(para.EncryptedData, customer.SessionKey, para.IV)
	userInfo := &object.HashMap{}
	err = object.JsonDecode(userData, userInfo)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_ACCOUNT_INFO, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	if (*userInfo)["nickName"] == nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_ACCOUNT_INFO, global.API_RETURN_CODE_ERROR, "", "nick name is empty")
		panic(ctl.RS)
		return
	}
	// ----

	customer.Name = (*userInfo)["nickName"].(string)

	serviceCustomer := service.NewCustomerService(context)
	customer, err = serviceCustomer.UpdateCustomer(globalDatabase.G_DBConnection, customer, true)

	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_UPSERT_ACCOUNT, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, customer)

}
