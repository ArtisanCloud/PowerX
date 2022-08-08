package tag

import (
	"errors"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type ParaBindTagsToCustomerToEmployeeByContactWayTags struct {
	CustomerExternalUserID string `form:"customerExternalUserID" json:"customerExternalUserID" binding:"required"`
	EmployeeWXUserID       string `form:"employeeWXUserID" json:"employeeWXUserID" binding:"required"`
	ContactWayConfigID     string `form:"contactWayConfigID" json:"contactWayConfigID" binding:"required"`
}

func ValidateBindTagsToCustomerToEmployeeByContactWayTags(context *gin.Context) {
	var form ParaBindTagsToCustomerToEmployeeByContactWayTags

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)
	pivot, contactWay, err := convertParaToBindTagsToCustomerToEmployeeByContactWayTags(&form)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
	}
	context.Set("pivot", pivot)
	context.Set("contactWay", contactWay)
	context.Next()
}

func convertParaToBindTagsToCustomerToEmployeeByContactWayTags(form *ParaBindTagsToCustomerToEmployeeByContactWayTags) (pivot *models.RCustomerToEmployee, contactWay *models.ContactWay, err error) {

	pivot, err = (&models.RCustomerToEmployee{}).GetPivot(global.DBConnection, form.CustomerExternalUserID, form.EmployeeWXUserID)
	if err != nil {
		return nil, nil, err
	}
	if pivot == nil {
		return nil, nil, errors.New("pivot not found")
	}

	serviceContactWay := service.NewContactWayService(nil)
	contactWay, err = serviceContactWay.GetContactWayByConfigID(global.DBConnection, form.ContactWayConfigID)
	if contactWay == nil {
		return pivot, contactWay, errors.New("contactWay not found")
	}

	contactWay.WXTags, err = contactWay.LoadWXTags(global.DBConnection, nil)
	if err != nil {
		return pivot, contactWay, err
	}
	if len(contactWay.WXTags) == 0 {
		return pivot, contactWay, errors.New("contactWay has no wxTags")
	}

	return pivot, contactWay, err
}
