package tag

import (
	"errors"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	serviceWX "github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type ParaBindTagsToCustomerToEmployee struct {
	CustomerExternalUserID string   `form:"customerExternalUserID" json:"customerExternalUserID" binding:"required"`
	EmployeeWXUserID       string   `form:"employeeWXUserID" json:"employeeWXUserID" binding:"required"`
	TagWXIDs               []string `form:"tagWXIDs" json:"tagWXIDs" binding:"required,min=1"`
}

func ValidateBindTagsToCustomerToEmployee(context *gin.Context) {
	var form ParaBindTagsToCustomerToEmployee

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)
	pivot, wxTags, err := convertParaToBindTagsToCustomerToEmployee(&form)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
	}
	context.Set("pivot", pivot)
	context.Set("tags", wxTags)
	context.Next()
}

func convertParaToBindTagsToCustomerToEmployee(form *ParaBindTagsToCustomerToEmployee) (pivot *models.RCustomerToEmployee, wxTags []*wx.WXTag, err error) {

	pivot, err = (&models.RCustomerToEmployee{}).GetPivot(global.DBConnection, form.CustomerExternalUserID, form.EmployeeWXUserID)
	if err != nil {
		return nil, nil, err
	}
	if pivot == nil {
		return nil, nil, errors.New("pivot not found")
	}

	serviceWXTag := serviceWX.NewWXTagService(nil)
	wxTags, err = serviceWXTag.GetWXTagsByIDs(global.DBConnection, form.TagWXIDs)

	return pivot, wxTags, err
}
