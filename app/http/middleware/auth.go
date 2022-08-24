package middleware

import (
	modelPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/models"
	service "github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	globalRBAC "github.com/ArtisanCloud/PowerX/boostrap/rbac/global"
	globalConfig "github.com/ArtisanCloud/PowerX/configs/global"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

func AuthCustomerAPI(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	strAuthorization := c.GetHeader("Authorization")

	if strAuthorization == "" {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_TOKEN_NOT_IN_HEADER, globalConfig.API_RETURN_CODE_ERROR, "", "")

	} else {
		var (
			customer *models.Customer
			err      error
		)
		ptrClaims, err := service.ParseAuthorization(strAuthorization)
		if ptrClaims == nil || err != nil {
			apiResponse.SetCode(globalConfig.API_ERR_CODE_ACCOUNT_INVALID_TOKEN, globalConfig.API_RETURN_CODE_ERROR, "", "")
			apiResponse.ThrowJSONResponse(c)
			return
		}
		claims := *ptrClaims
		if claims["OpenID"] == nil && claims["ExternalUserID"] == nil {
			apiResponse.SetCode(globalConfig.API_ERR_CODE_ACCOUNT_INVALID_TOKEN, globalConfig.API_RETURN_CODE_ERROR, "", "")
		} else {
			serviceWeComCustomer := wecom.NewWeComCustomerService(c)
			if claims["OpenID"] != nil {
				openID := claims["OpenID"].(string)
				if openID == "" {
					apiResponse.SetCode(globalConfig.API_ERR_CODE_LACK_OF_WX_EXTERNAL_USER_ID, globalConfig.API_RETURN_CODE_ERROR, "", "")
				}
				customer, err = serviceWeComCustomer.GetCustomerByOpenID(global.G_DBConnection, openID)

				// set auth open id
				wecom.SetAuthOpenID(c, openID)

			} else if claims["ExternalUserID"] != nil {
				externalUserID := claims["ExternalUserID"].(string)
				if externalUserID == "" {
					apiResponse.SetCode(globalConfig.API_ERR_CODE_LACK_OF_WX_EXTERNAL_USER_ID, globalConfig.API_RETURN_CODE_ERROR, "", "")
				}
				customer, err = serviceWeComCustomer.GetCustomerByWXExternalUserID(global.G_DBConnection, externalUserID)
			}

			if err != nil || customer.PowerModel == nil {
				apiResponse.SetCode(globalConfig.API_ERR_CODE_ACCOUNT_UNREGISTER, globalConfig.API_RETURN_CODE_ERROR, "", "")
			} else {
				service.SetAuthCustomer(c, customer)
			}

		}
	}

	if !apiResponse.IsNoError() {
		apiResponse.ThrowJSONResponse(c)
	}
	return

}

func AuthenticateEmployeeAPI(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)
	var (
		employee *models.Employee
		err      error
	)

	// 获取token
	strAuthorization := c.GetHeader("Authorization")
	if strAuthorization == "" {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_TOKEN_NOT_IN_HEADER, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	// 解析jwt token信息
	ptrClaims, err := service.ParseAuthorization(strAuthorization)
	if ptrClaims == nil || err != nil {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_ACCOUNT_INVALID_TOKEN, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}
	claims := *ptrClaims
	if claims["WXUserID"] == nil {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_LACK_OF_WX_USER_ID, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}
	wxUserID := claims["WXUserID"].(string)
	if err != nil || wxUserID == "" {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_LACK_OF_WX_USER_ID, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	// 获取企业员工身份
	serviceWeComEmployee := wecom.NewWeComEmployeeService(c)
	employee, err = serviceWeComEmployee.GetEmployeeByUserID(global.G_DBConnection, wxUserID)
	if err != nil || employee == nil {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_EMPLOYEE_UNREGISTER, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	service.SetAuthEmployee(c, employee)

	return

}

func AuthRootAPI(c *gin.Context) {

}

// ------------------------------------------------------------------------------------------------------------------------------------------------

func AuthorizeAPI(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	serviceRBAC := service.NewRBACService(c)
	permission, err := serviceRBAC.GetCachedPermissionByResource(global.G_DBConnection, c.Request.URL.Path, c.Request.Method)

	employee := service.GetAuthEmployee(c)

	// 验证接口的访问权限
	isPass, err := globalRBAC.Enforcer.Enforce(employee.Role.GetRBACRuleName(), permission.PermissionModule.GetRBACRuleName(), modelPowerLib.RBAC_CONTROL_ALL)
	if err != nil {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_AUTHORIZATE_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		apiResponse.ThrowJSONResponse(c)
		return

	}
	// 传递结果
	if isPass {
		c.Next()
	} else {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_AUTHORIZATE_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

}
