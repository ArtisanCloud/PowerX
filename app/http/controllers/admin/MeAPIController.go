package admin

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/gin-gonic/gin"
)

type MeAPIController struct {
	*api.APIController
	ServiceMe *service.EmployeeService
}

func NewMeAPIController(context *gin.Context) (ctl *MeAPIController) {

	return &MeAPIController{
		APIController: api.NewAPIController(context),
		ServiceMe:     service.NewEmployeeService(context),
	}
}

func APIMeDetail(context *gin.Context) {

	ctl := NewMeAPIController(context)

	defer api.RecoverResponse(context, "api.admin.me.detail")

	me := service.GetAuthEmployee(context)

	ctl.RS.Success(context, me)

}
