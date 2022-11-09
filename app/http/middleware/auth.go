package middleware

import (
	modelPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/models"
	service "github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/weCom"
	globalRBAC "github.com/ArtisanCloud/PowerX/boostrap/rbac/global"
	globalConfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

func AuthCustomerByHeader(c *gin.Context) {

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
			serviceWeComCustomer := weCom.NewWeComCustomerService(c)
			if claims["OpenID"] != nil {
				openID := claims["OpenID"].(string)
				if openID == "" {
					apiResponse.SetCode(globalConfig.API_ERR_CODE_LACK_OF_WX_EXTERNAL_USER_ID, globalConfig.API_RETURN_CODE_ERROR, "", "")
				}
				customer, err = serviceWeComCustomer.GetCustomerByOpenID(global.G_DBConnection, openID)

				// set auth open id
				weCom.SetAuthOpenID(c, openID)

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

func AuthenticateEmployeeByQuery(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	// 获取token
	strToken := c.Query("token")
	if strToken == "" {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_TOKEN_NOT_IN_QUERY, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	resultCode := AuthenticateEmployee(c, strToken)
	if resultCode != globalConfig.API_RESULT_CODE_INIT {
		apiResponse.SetCode(resultCode, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	return
}

func AuthenticateEmployeeByHeader(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	// 获取token
	strToken := c.GetHeader("Authorization")
	if strToken == "" {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_TOKEN_NOT_IN_HEADER, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	resultCode := AuthenticateEmployee(c, strToken)
	if resultCode != globalConfig.API_RESULT_CODE_INIT {
		apiResponse.SetCode(resultCode, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	return
}

func AuthenticateRootByHeader(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	// 获取token
	strToken := c.GetHeader("Authorization")
	if strToken == "" {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_TOKEN_NOT_IN_HEADER, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	resultCode := AuthenticateRoot(c, strToken)
	if resultCode != globalConfig.API_RESULT_CODE_INIT {
		apiResponse.SetCode(resultCode, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

	return
}

func ParseUserByToken(c *gin.Context, strToken string) (wxUserID string, errCode int) {
	// 解析jwt token信息
	ptrClaims, err := service.ParseAuthorization(strToken)
	if ptrClaims == nil || err != nil {
		return wxUserID, globalConfig.API_ERR_CODE_ACCOUNT_INVALID_TOKEN
	}
	claims := *ptrClaims
	if claims["WXUserID"] == nil {
		return wxUserID, globalConfig.API_ERR_CODE_LACK_OF_WX_USER_ID
	}
	wxUserID = claims["WXUserID"].(string)
	if err != nil || wxUserID == "" {
		return wxUserID, globalConfig.API_ERR_CODE_LACK_OF_WX_USER_ID
	}

	return wxUserID, errCode
}

func AuthenticateEmployee(c *gin.Context, strToken string) (errCode int) {

	wxUserID, errCode := ParseUserByToken(c, strToken)
	if errCode != globalConfig.API_RESULT_CODE_INIT {
		return errCode
	}

	// 获取企业员工身份
	serviceWeComEmployee := weCom.NewWeComEmployeeService(c)
	employee, err := serviceWeComEmployee.GetEmployeeByUserID(global.G_DBConnection, wxUserID)
	if err != nil || employee == nil {
		return globalConfig.API_ERR_CODE_EMPLOYEE_UNREGISTER
	}
	// 确认企业员工的微信状态是否被激活
	if !serviceWeComEmployee.IsActive(employee) {
		return globalConfig.API_ERR_CODE_EMPLOYEE_STATUS_NOT_ACTIVE
	}

	// 员工未分配角色
	if employee.Role == nil && employee.RoleID == nil {
		return globalConfig.API_ERR_CODE_EMPLOYEE_HAS_NO_ROLE
	}

	// 确认员工是否有角色，否则视为未激活
	service.SetAuthEmployee(c, employee)

	return globalConfig.API_RESULT_CODE_INIT
}

func AuthenticateRoot(c *gin.Context, strToken string) (errCode int) {
	wxUserID, errCode := ParseUserByToken(c, strToken)
	if errCode != globalConfig.API_RESULT_CODE_INIT {
		return errCode
	}
	if errCode != globalConfig.API_RESULT_CODE_INIT {
		return errCode
	}

	// 获取Root身份
	serviceEmployee := service.NewEmployeeService(c)
	root, err := serviceEmployee.GetRoot(global.G_DBConnection)
	if err != nil || root == nil {
		return globalConfig.API_ERR_CODE_FAIL_TO_GET_ROOT
	}

	// 员工未分配角色
	if root.WXUserID.String == "" || root.WXUserID.String != wxUserID {
		return globalConfig.API_ERR_CODE_CURRENT_LOGIN_IS_NOT_ROOT
	}

	// 确认员工是否有角色，否则视为未激活
	service.SetAuthEmployee(c, root)

	return globalConfig.API_RESULT_CODE_INIT
}

// ------------------------------------------------------------------------------------------------------------------------------------------------

func AuthorizeAPI(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	serviceRBAC := service.NewRBACService(c)
	permission, err := serviceRBAC.GetCachedPermissionByResource(global.G_DBConnection, c.Request.URL.Path, c.Request.Method)

	employee := service.GetAuthEmployee(c)
	// 员工未登陆
	if employee == nil {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		apiResponse.ThrowJSONResponse(c)
		return
	}

	// 员工未分配角色
	if employee.Role == nil && *employee.RoleID == "" {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_EMPLOYEE_HAS_NO_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		apiResponse.ThrowJSONResponse(c)
		return
	}

	// 该接口未被分配权限控制
	isPass := false
	if permission == nil || permission.PermissionModule == nil {
		c.Next()
		return
	}
	// 验证接口的访问权限
	isPass, err = globalRBAC.G_Enforcer.Enforce(employee.Role.GetRBACRuleName(), permission.PermissionModule.GetRBACRuleName(), modelPowerLib.RBAC_CONTROL_ALL)
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
