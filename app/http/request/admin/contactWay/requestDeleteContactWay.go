package contactWay

import (
	"errors"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/configs/global"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type ParaDeleteContactWay struct {
	ConfigID string `form:"configID" json:"configID" xml:"configID" binding:"required"`
}

func ValidateDeleteContactWay(context *gin.Context) {
	var form ParaDeleteContactWay

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)

	contactWay, err := convertParaDeleteContactWayForDelete(&form)
	if err != nil {
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
	}

	context.Set("contactWay", contactWay)
	context.Next()
}

func convertParaDeleteContactWayForDelete(form *ParaDeleteContactWay) (contactWay *models.ContactWay, err error) {

	serviceContactWay := service.NewContactWayService(nil)
	contactWay, err = serviceContactWay.GetContactWayByConfigID(globalDatabase.G_DBConnection, form.ConfigID)

	if err != nil {
		return contactWay, err
	}

	if contactWay == nil {
		return contactWay, errors.New("contactWay is nil")
	}

	return contactWay, nil

}
