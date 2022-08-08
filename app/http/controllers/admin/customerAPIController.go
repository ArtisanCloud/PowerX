package admin

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/service"
	global2 "github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type CustomerAPIController struct {
	*api.APIController
	ServiceCustomer *service.CustomerService
}

func NewCustomerAPIController(context *gin.Context) (ctl *CustomerAPIController) {

	return &CustomerAPIController{
		APIController:   api.NewAPIController(context),
		ServiceCustomer: service.NewCustomerService(context),
	}
}

func APIWXCustomerSync(context *gin.Context) {
	ctl := NewCustomerAPIController(context)

	employeeUserIDsInterface, _ := context.Get("employeeUserIDs")
	employeeUserIDs := employeeUserIDsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.wecom.customer.upsert")

	var err error
	err = ctl.ServiceCustomer.SyncCustomers(employeeUserIDs, "")

	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_UPSERT_ACCOUNT, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)

}

func APIGetCustomerList(context *gin.Context) {
	ctl := NewCustomerAPIController(context)

	params, _ := context.Get("params")
	para := params.(request.ParaList)

	defer api.RecoverResponse(context, "api.admin.customer.list")

	arrayList, err := ctl.ServiceCustomer.GetList(global.DBConnection, nil, para.Page, para.PageSize)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_ACCOUNT_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetCustomerDetail(context *gin.Context) {
	ctl := NewCustomerAPIController(context)

	externalUserIDInterface, _ := context.Get("externalUserID")
	externalUserID := externalUserIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.customer.detail")

	account, err := ctl.ServiceCustomer.GetCustomerByExternalUserID(global.DBConnection, externalUserID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_ACCOUNT_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, account)
}

// ----------------------------- wx platform   -------------------------------------------------
func APIGetCustomerListOnWXPlatform(context *gin.Context) {
	ctl := NewCustomerAPIController(context)

	userIDInterface, _ := context.Get("userID")
	userID := userIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.employee.list")

	arrayList, err := global2.WeComCustomer.App.ExternalContact.List(userID)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetCustomerDetailOnWXPlatform(context *gin.Context) {
	ctl := NewCustomerAPIController(context)

	externalUserIDInterface, _ := context.Get("externalUserID")
	externalUserID := externalUserIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.employee.detail")

	result, err := global2.WeComCustomer.App.ExternalContact.Get(externalUserID, "")
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, result)
}
