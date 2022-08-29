package wx

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerSocialite/v2/src/providers"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/contract"
	modelPowerWechat "github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/models"
	modelWecomEvent "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/config"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"net/http"
)

type WeComAPIController struct {
	*api.APIController
}

func NewWeComAPIController(context *gin.Context) (ctl *WeComAPIController) {

	return &WeComAPIController{
		APIController: api.NewAPIController(context),
	}
}

// ------------------------ common APIs --------------------------------
func APIGetCallbackIPs(context *gin.Context) {
	ctl := NewWeComAPIController(context)

	ips, _ := wecom.G_WeComApp.App.Base.GetCallbackIP()

	ctl.RS.Success(context, ips)
}

// ------------------------ wecom employee APIs --------------------------------
func APICallbackValidationEmployee(context *gin.Context) {
	ctl := NewWeComAPIController(context)

	response, err := wecom.G_WeComEmployee.App.Server.Serve(context.Request)
	if err != nil {
		ctl.RS.Error(context, config.API_RETURN_CODE_ERROR, err.Error(), "")
		return
	}

	text, err := ioutil.ReadAll(response.Body)

	context.String(200, string(text))

}

// https://developer.work.weixin.qq.com/document/path/90967
func APICallbackEmployee(context *gin.Context) {
	ctl := NewWeComAPIController(context)

	//requestXML, _ := ioutil.ReadAll(context.Request.Body)
	//context.Request.Body = ioutil.NopCloser(bytes.NewBuffer(requestXML))
	//println(string(requestXML))

	rs, err := wecom.G_WeComEmployee.App.Server.Notify(context.Request, func(event contract.EventInterface) (result interface{}) {

		result = kernel.SUCCESS_EMPTY_RESPONSE

		switch event.GetMsgType() {
		case modelPowerWechat.CALLBACK_MSG_TYPE_EVENT:
			{
				ctl.HandleEmployeeEvent(context, event)
			}
			break
		case modelPowerWechat.CALLBACK_MSG_TYPE_TEXT:
		case modelPowerWechat.CALLBACK_MSG_TYPE_IMAGE:
		case modelPowerWechat.CALLBACK_MSG_TYPE_VOICE:
		case modelPowerWechat.CALLBACK_MSG_TYPE_VIDEO:
		case modelPowerWechat.CALLBACK_MSG_TYPE_LOCATION:
		case modelPowerWechat.CALLBACK_MSG_TYPE_LINK:

		default:

		}
		return result
	})

	if err != nil {
		panic(err)
	}

	err = rs.Send(context.Writer)
	if err != nil {
		panic(err)
	}
}

// ------------------------ wecom employee events --------------------------------
func (ctl *WeComAPIController) HandleEmployeeEvent(context *gin.Context, event contract.EventInterface) (result interface{}) {

	result = kernel.SUCCESS_EMPTY_RESPONSE

	var err error
	switch event.GetEvent() {
	case modelWecomEvent.CALLBACK_EVENT_CHANGE_CONTACT:
		{
			err = ctl.HandleEventChangeContact(context, event)
			break
		}

	case modelWecomEvent.CALLBACK_EVENT_SUBSCRIBE:
	case modelWecomEvent.CALLBACK_EVENT_ENTER_AGENT:
	case modelWecomEvent.CALLBACK_EVENT_LOCATION:
	case modelWecomEvent.CALLBACK_EVENT_BATCH_JOB_RESULT:
	case modelWecomEvent.CALLBACK_EVENT_CLICK:
	case modelWecomEvent.CALLBACK_EVENT_VIEW:
	case modelWecomEvent.CALLBACK_EVENT_SCANCODE_PUSH:
	case modelWecomEvent.CALLBACK_EVENT_SCANCODE_WAITMSG:
	case modelWecomEvent.CALLBACK_EVENT_PIC_SYSPHOTO:
	case modelWecomEvent.CALLBACK_EVENT_PIC_PHOTO_OR_ALBUM:
	case modelWecomEvent.CALLBACK_EVENT_PIC_WEIXIN:

	default:

	}

	if err != nil {
		wecom.G_WeComEmployee.App.Logger.Error(err.Error())
		return err.Error()
	}

	return result
}

// ------------------------ wecom customer APIs --------------------------------
func APICallbackValidationCustomer(context *gin.Context) {
	ctl := NewWeComAPIController(context)

	response, err := wecom.G_WeComCustomer.App.Server.Serve(context.Request)
	if err != nil {
		ctl.RS.Error(context, config.API_RETURN_CODE_ERROR, err.Error(), "")
		return
	}

	text, err := ioutil.ReadAll(response.Body)

	context.String(200, string(text))

}

// // https://developer.work.weixin.qq.com/document/path/92129
func APICallbackCustomer(context *gin.Context) {
	ctl := NewWeComAPIController(context)

	//requestXML, _ := ioutil.ReadAll(context.Request.Body)
	//context.Request.Body = ioutil.NopCloser(bytes.NewBuffer(requestXML))
	//println(string(requestXML))

	rs, err := wecom.G_WeComEmployee.App.Server.Notify(context.Request, func(event contract.EventInterface) (result interface{}) {

		result = kernel.SUCCESS_EMPTY_RESPONSE

		switch event.GetMsgType() {
		case modelPowerWechat.CALLBACK_MSG_TYPE_EVENT:
			{
				ctl.HandleCustomerEvent(context, event)
			}
			break
		case modelPowerWechat.CALLBACK_MSG_TYPE_TEXT:
		case modelPowerWechat.CALLBACK_MSG_TYPE_IMAGE:
		case modelPowerWechat.CALLBACK_MSG_TYPE_VOICE:
		case modelPowerWechat.CALLBACK_MSG_TYPE_VIDEO:
		case modelPowerWechat.CALLBACK_MSG_TYPE_LOCATION:
		case modelPowerWechat.CALLBACK_MSG_TYPE_LINK:

		default:

		}
		return result
	})

	if err != nil {
		panic(err)
	}

	err = rs.Send(context.Writer)
	if err != nil {
		panic(err)
	}

}

// ------------------------ wecom customer events --------------------------------
func (ctl *WeComAPIController) HandleCustomerEvent(context *gin.Context, event contract.EventInterface) (result interface{}) {

	result = kernel.SUCCESS_EMPTY_RESPONSE

	var err error
	switch event.GetEvent() {
	case modelWecomEvent.CALLBACK_EVENT_CHANGE_EXTERNAL_CONTACT:
		{
			err = ctl.HandleEventChangeCustomer(context, event)
			break
		}
	case modelWecomEvent.CALLBACK_EVENT_CHANGE_EXTERNAL_CHAT:
		{
			err = ctl.HandleEventChangeExternalChat(context, event)
			break
		}
	case modelWecomEvent.CALLBACK_EVENT_CHANGE_EXTERNAL_TAG:
		{
			err = ctl.HandleEventChangeExternalTag(context, event)
			break
		}
	}

	if err != nil {
		wecom.G_WeComCustomer.App.Logger.Error(err.Error())
		return err.Error()
	}

	return result
}

func (ctl *WeComAPIController) HandleEventChangeCustomer(context *gin.Context, event contract.EventInterface) (err error) {

	serviceEmployee := service.NewEmployeeService(context)
	switch event.GetChangeType() {
	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_ADD_EXTERNAL_CONTACT:
		err = serviceEmployee.HandleAddCustomer(context, event)
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_EDIT_EXTERNAL_CONTACT:
		err = serviceEmployee.HandleEditCustomer(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_ADD_HALF_EXTERNAL_CONTACT:
		err = serviceEmployee.HandleAddHalfCustomer(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_DEL_EXTERNAL_CONTACT:
		err = serviceEmployee.HandleDelCustomer(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_DEL_FOLLOW_USER:
		msg, err := serviceEmployee.HandleDelFollowEmployee(context, event)
		if err != nil {
			return err
		}

		err = wecom.G_WeComCustomer.SendDelCustomerWelcomeMsg(context, msg)
		if err != nil {
			return err
		}

		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_TRANSFER_FAIL:
		err = serviceEmployee.HandleTransferFail(context, event)
		if err != nil {
			return err
		}
		break

	}

	return err
}

// ------------------------ wecom employee and customer events --------------------------------
func (ctl *WeComAPIController) HandleEventChangeExternalChat(context *gin.Context, event contract.EventInterface) (err error) {

	serviceGroupChat := service.NewGroupChatService(context)
	switch event.GetChangeType() {

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_CREATE:
		err = serviceGroupChat.HandleChatCreate(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_UPDATE:
		err = serviceGroupChat.HandleChatUpdate(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_DISMISS:
		err = serviceGroupChat.HandleChatDismiss(context, event)
		if err != nil {
			return err
		}
		break

	}
	return err
}

func (ctl *WeComAPIController) HandleEventChangeExternalTag(context *gin.Context, event contract.EventInterface) (err error) {

	serviceTag := wecom.NewWXTagService(context)

	switch event.GetChangeType() {

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_CREATE:
		err = serviceTag.HandleTagCreate(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_UPDATE:
		err = serviceTag.HandleTagUpdate(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_DELETE:
		err = serviceTag.HandleTagDelete(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_SHUFFLE:
		err = serviceTag.HandleTagShuffle(context, event)
		if err != nil {
			return err
		}
		break
	}

	return err
}

func (ctl *WeComAPIController) HandleEventChangeContact(context *gin.Context, event contract.EventInterface) (err error) {

	serviceEmployee := service.NewEmployeeService(context)
	serviceDepartment := service.NewDepartmentService(context)
	switch event.GetChangeType() {

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_CREATE_USER:
		err = serviceEmployee.HandleEmployeeCreate(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_UPDATE_USER:
		err = serviceEmployee.HandleEmployeeUpdate(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_DELETE_USER:
		err = serviceEmployee.HandleEmployeeDelete(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_CREATE_PARTY:
		err = serviceDepartment.HandleDepartmentCreate(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_UPDATE_PARTY:
		err = serviceDepartment.HandleDepartmentUpdate(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_DELETE_PARTY:
		err = serviceDepartment.HandleDepartmentDelete(context, event)
		if err != nil {
			return err
		}
		break

	case modelWecomEvent.CALLBACK_EVENT_CHANGE_TYPE_UPDATE_TAG:
		err = serviceEmployee.HandleContactTagUpdate(context, event)
		if err != nil {
			return err
		}
		break

	default:

	}
	return err
}

// ---------------------------------------------------------------------------------------------------------------------
func WeComToAuthorizeCustomer(context *gin.Context) {

	wecom.G_WeComCustomer.Authorize(context, "/callback/authorized/customer")

}
func WeComToAuthorizeEmployee(context *gin.Context) {

	wecom.G_WeComCustomer.Authorize(context, "/callback/authorized/employee")

}
func WeComToAuthorizeQREmployee(context *gin.Context) {

	wecom.G_WeComEmployee.AuthorizeQR(context, "/callback/authorized/qr/employee")

}

func WeComAuthorizedCustomer(context *gin.Context) {
	ctl := NewWeComAPIController(context)

	// get customer info from code
	customer, err := wecom.G_WeComCustomer.AuthorizedCustomer(context)
	if err != nil {
		ctl.RS.SetCode(http.StatusExpectationFailed, config.API_RETURN_CODE_ERROR, "", err.Error())
		ctl.RS.ThrowJSONResponse(context)
		return
	}

	var account *models.Customer
	// query contract detail by user external id
	// **** Customer have to be  externalEmployeeID and openID
	externalEmployeeID := customer.GetExternalUserID()
	openID := customer.GetOpenID()

	serviceCustomer := wecom.NewWeComCustomerService(context)
	customer, err = serviceCustomer.GetContactByExternalUserID(context, externalEmployeeID)

	// query user detail by user id
	workConfig := wecom.G_WeComCustomer.App.GetConfig()
	corpID := workConfig.GetString("corp_id", "")
	appID := corpID

	customer.SetAttribute("openID", openID)
	// wechat work external customer
	if externalEmployeeID != "" && corpID != "" && openID != "" {
		customer.SetAttribute("corpID", corpID)
		customer.SetAttribute("external_contact.external_userid", externalEmployeeID)

	} else
	// official or mini program customer
	if appID != "" && openID != "" {
		customer.SetAttribute("appID", appID)

	} else {
		// invalid wechat customer
		ctl.RS.SetCode(http.StatusExpectationFailed, config.API_RETURN_CODE_ERROR, "", "invalid wechat customer")
		ctl.RS.ThrowJSONResponse(context)
		return
	}

	account = models.NewCustomer(object.NewCollection(customer.GetAttributes()))
	err = service.NewCustomerService(context).UpsertCustomers(globalDatabase.G_DBConnection, []*models.Customer{account}, nil)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_ACCOUNT, config.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	serviceAuth := service.NewAuthService(context)
	strToken, _ := serviceAuth.CreateTokenForCustomer(account)

	res := map[string]interface{}{
		"token_type":    "Bearer",
		"expires_in":    service.InExpiredSecond,
		"access_token":  strToken,
		"refresh_token": "",
	}

	// 正常返回json
	ctl.RS.Success(context, res)
}

func WeComAuthorizedEmployee(context *gin.Context) {
	ctl := NewWeComAPIController(context)

	// get user info from code
	user, err := wecom.G_WeComEmployee.AuthorizedEmployee(context)
	if err != nil {
		ctl.RS.SetCode(http.StatusExpectationFailed, config.API_RETURN_CODE_ERROR, "", err.Error())
		ctl.RS.ThrowJSONResponse(context)
		return
	}

	strToken, rsCode := WeComGetEmployeeToken(context, user)
	if rsCode != config.API_RESULT_CODE_INIT {
		ctl.RS.SetCode(rsCode, config.API_RETURN_CODE_ERROR, "", "")
		ctl.RS.ThrowJSONResponse(context)
		return
	}
	res := map[string]interface{}{
		"token_type":    "Bearer",
		"expires_in":    service.InExpiredSecond,
		"access_token":  strToken,
		"refresh_token": "",
	}

	//// 正常返回json
	ctl.RS.Success(context, res)
}

func WeComAuthorizedEmployeeQR(context *gin.Context) {
	ctl := NewWeComAPIController(context)

	// get user info from code
	user, err := wecom.G_WeComEmployee.AuthorizedEmployeeQR(context)
	if err != nil {
		ctl.RS.SetCode(http.StatusExpectationFailed, config.API_RETURN_CODE_ERROR, "", err.Error())
		ctl.RS.ThrowJSONResponse(context)
		return
	}

	strToken, rsCode := WeComGetEmployeeToken(context, user)
	if rsCode != config.API_RESULT_CODE_INIT {
		ctl.RS.SetCode(rsCode, config.API_RETURN_CODE_ERROR, "", "")
		ctl.RS.ThrowJSONResponse(context)
		return
	}

	res := map[string]interface{}{
		"token_type":    "Bearer",
		"expires_in":    service.InExpiredSecond,
		"access_token":  strToken,
		"refresh_token": "",
	}

	//// 正常返回json
	ctl.RS.Success(context, res)
}

func WeComGetEmployeeToken(context *gin.Context, user *providers.User) (strToken string, rsCode int) {
	var employee *models.Employee
	userID := user.GetID()
	if userID == "" {
		return "", config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL
	}

	serviceEmployee := service.NewEmployeeService(context)
	employee, _ = serviceEmployee.GetEmployeeByUserIDOnWXPlatform(context, userID)

	// query user detail by user id
	if user.GetOpenID() == "" {
		responseOpenID, err := wecom.G_WeComEmployee.App.User.UserIdToOpenID(userID)
		if err != nil || responseOpenID.OpenID == "" {
			return "", config.API_ERR_CODE_LACK_OF_WX_OPEN_ID
		}
		employee.WXEmployee.WXCorpID = object.NewNullString(wecom.G_WeComEmployee.App.Config.GetString("corp_id", ""), true)
		employee.WXEmployee.WXOpenID = object.NewNullString(responseOpenID.OpenID, true)
	}
	serviceWeComEmployee := wecom.NewWeComEmployeeService(nil)
	err := serviceWeComEmployee.UpsertEmployeeByWXEmployee(globalDatabase.G_DBConnection, employee.WXEmployee)
	if err != nil {
		return "", config.API_ERR_CODE_FAIL_TO_UPSERT_EMPLOYEE

	}

	serviceAuth := service.NewAuthService(context)
	strToken, _ = serviceAuth.CreateTokenForEmployee(employee)

	return strToken, config.API_RESULT_CODE_INIT
}
