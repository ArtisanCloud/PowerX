package employee

import (
	"errors"
	models2 "github.com/ArtisanCloud/PowerSocialite/v2/src/models"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config/global"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type ParaBindCustomerToEmployee struct {
	CustomerExternalUserID string `form:"customerExternalUserID" json:"customerExternalUserID" binding:"required"`
	EmployeeWXUserID       string `form:"employeeWXUserID" json:"employeeWXUserID" binding:"required"`

	UserID         string   `form:"userID" json:"userID"`
	Remark         string   `form:"remark" json:"remark"`
	Description    string   `form:"description" json:"description"`
	CreateTime     int      `form:"createTime" json:"createTime"`
	TagIDs         []string `form:"tagIDs" json:"tagIDs"`
	RemarkCorpName string   `form:"remarkCorpName" json:"remarkCorpName"`
	RemarkMobiles  []string `form:"remarkMobiles" json:"remarkMobiles"`
	OperUserID     string   `form:"operUserID" json:"operUserID"`
	AddWay         int      `form:"addWay" json:"addWay"`
	State          string   `form:"state" json:"state"`
}

func ValidateBindCustomerToEmployee(context *gin.Context) {
	var form ParaBindCustomerToEmployee

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)
	customer, employee, followInfo, err := convertParaToBindCustomerToEmployee(&form)
	if err != nil {
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}
	context.Set("customer", customer)
	context.Set("employee", employee)
	context.Set("followInfo", followInfo)
	context.Next()
}

func convertParaToBindCustomerToEmployee(form *ParaBindCustomerToEmployee) (customer *models.Customer, employee *models.Employee, followInfo *models2.FollowUser, err error) {

	serviceCustomer := service.NewCustomerService(nil)
	customer, err = serviceCustomer.GetCustomerByExternalUserID(globalDatabase.G_DBConnection, form.CustomerExternalUserID)
	if err != nil {
		return customer, employee, followInfo, err
	}
	if customer == nil {
		return customer, employee, followInfo, errors.New("customer not found")
	}

	serviceEmployee := service.NewEmployeeService(nil)
	employee, err = serviceEmployee.GetEmployeeByUserID(globalDatabase.G_DBConnection, form.EmployeeWXUserID)
	if err != nil {
		return customer, employee, followInfo, err
	}
	if employee == nil {
		return customer, employee, followInfo, errors.New("employee not found")
	}

	followInfo = &models2.FollowUser{
		UserID:         form.UserID,
		Remark:         form.Remark,
		Description:    form.Description,
		CreateTime:     form.CreateTime,
		TagIDs:         form.TagIDs,
		RemarkCorpName: form.RemarkCorpName,
		RemarkMobiles:  form.RemarkMobiles,
		OperUserID:     form.OperUserID,
		AddWay:         form.AddWay,
		State:          form.State,
	}

	return customer, employee, followInfo, err
}
