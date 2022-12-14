package root

import (
	models2 "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/helper"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type ParaRegisterRoot struct {
	Email    string `form:"email" json:"email"  binding:"required"`
	Password string `form:"password" json:"password"  binding:"required"`
}

func ValidateRegisterRoot(context *gin.Context) {
	var form ParaRegisterRoot

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	apiResponse := http.NewAPIResponse(context)

	// 检查是否系统已经有root被初始化过
	serviceInstall := service.NewInstallService(context)
	rootExisted, err := serviceInstall.CheckRootInitialization(context)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}

	if rootExisted != nil {
		apiResponse.SetCode(config.API_ERR_CODE_ROOT_HAS_BEEN_INITIALIZED, config.API_RETURN_CODE_ERROR, "", "").ThrowJSONResponse(context)
		return
	}

	rootEmployee := models.NewEmployee(object.NewCollection(&object.HashMap{
		"email":    form.Email,
		"password": helper.EncodePassword(form.Password),
		"roleID":   (&models2.Role{}).GetRootComposedUniqueID(),
	}))

	if rootEmployee == nil {
		apiResponse.SetCode(config.API_ERR_CODE_FAIL_TO_CREATE_EMPLOYEE, config.API_RETURN_CODE_ERROR, "", "").ThrowJSONResponse(context)
		return
	}

	context.Set("rootEmployee", rootEmployee)
	context.Next()
}
