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

	// ??????token
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

	// ??????token
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

	// ??????token
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

func ParseUserIDByToken(c *gin.Context, strToken string) (uniqueID string, errCode int) {
	// ??????jwt token??????
	ptrClaims, err := service.ParseAuthorization(strToken)
	if ptrClaims == nil || err != nil {
		return uniqueID, globalConfig.API_ERR_CODE_ACCOUNT_INVALID_TOKEN
	}
	claims := *ptrClaims
	if claims["EmployeeID"] == nil {
		return uniqueID, globalConfig.API_ERR_CODE_LACK_OF_EMPLOYEE_ID
	}
	uniqueID = claims["EmployeeID"].(string)
	if err != nil || uniqueID == "" {
		return uniqueID, globalConfig.API_ERR_CODE_LACK_OF_EMPLOYEE_ID
	}

	return uniqueID, errCode
}

func AuthenticateEmployee(c *gin.Context, strToken string) (errCode int) {

	employeeID, errCode := ParseUserIDByToken(c, strToken)
	if errCode != globalConfig.API_RESULT_CODE_INIT {
		return errCode
	}

	// ????????????????????????
	serviceEmployee := service.NewEmployeeService(c)
	employee, err := serviceEmployee.GetEmployeeByEmployeeID(global.G_DBConnection, employeeID)
	if err != nil || employee == nil {
		return globalConfig.API_ERR_CODE_EMPLOYEE_UNREGISTER
	}
	// ????????????????????? ??????????????????????????????????????????
	if !serviceEmployee.IsActive(employee) {
		return globalConfig.API_ERR_CODE_EMPLOYEE_HAS_NO_ROLE
	}

	// ???????????????????????????????????????????????????
	service.SetAuthEmployee(c, employee)

	return globalConfig.API_RESULT_CODE_INIT
}

func AuthenticateRoot(c *gin.Context, strToken string) (errCode int) {
	employeeID, errCode := ParseUserIDByToken(c, strToken)
	if errCode != globalConfig.API_RESULT_CODE_INIT {
		return errCode
	}

	// ??????Root??????
	serviceEmployee := service.NewEmployeeService(c)
	root, err := serviceEmployee.GetRoot(global.G_DBConnection)
	if err != nil || root == nil {
		return globalConfig.API_ERR_CODE_FAIL_TO_GET_ROOT
	}

	// ?????????????????????
	if root.UniqueID == "" || root.UniqueID != employeeID {
		return globalConfig.API_ERR_CODE_CURRENT_LOGIN_IS_NOT_ROOT
	}

	// ???????????????????????????????????????????????????
	service.SetAuthEmployee(c, root)

	return globalConfig.API_RESULT_CODE_INIT
}

// ------------------------------------------------------------------------------------------------------------------------------------------------

func AuthorizeAPI(c *gin.Context) {

	apiResponse := http.NewAPIResponse(c)

	serviceRBAC := service.NewRBACService(c)
	permission, err := serviceRBAC.GetCachedPermissionByResource(global.G_DBConnection, c.Request.URL.Path, c.Request.Method)

	employee := service.GetAuthEmployee(c)
	// ???????????????
	if employee == nil {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		apiResponse.ThrowJSONResponse(c)
		return
	}

	serviceEmployee := service.NewEmployeeService(c)
	// ?????????????????????
	if !serviceEmployee.IsActive(employee) {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_EMPLOYEE_HAS_NO_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		apiResponse.ThrowJSONResponse(c)
		return
	}

	// ?????????????????????????????????
	isPass := false
	if permission == nil || permission.PermissionModule == nil {
		c.Next()
		return
	}
	// ???????????????????????????
	// ?????????????????????
	subject := employee.Role.GetRBACRuleName()
	// ??????????????????????????????ID
	object := permission.PermissionModule.GetRBACRuleName()
	action := modelPowerLib.RBAC_CONTROL_ALL
	isPass, err = globalRBAC.G_Enforcer.Enforce(subject, object, action)
	if err != nil {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_AUTHORIZATE_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		apiResponse.ThrowJSONResponse(c)
		return
	}
	// ????????????
	if isPass {
		c.Next()
	} else {
		apiResponse.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_AUTHORIZATE_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", "")
		apiResponse.ThrowJSONResponse(c)
		return
	}

}
