package policy

import (
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type ParaCreateRolePolicies struct {
	Name     string  `form:"name" json:"name"`
	ParentID *string `form:"parentID" json:"parentID"`

	Policies []*models.RolePolicy `form:"policies" json:"policies" binding:"required,min=1"`
}

func ValidateCreateRolePolicies(context *gin.Context) {
	var form ParaCreateRolePolicies

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	_, err = convertParaToRoleForInsert(&form)
	if err != nil {
		apiResponse := http.NewAPIResponse(context)
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
		return
	}
	context.Set("rolePolicies", &form)
	context.Next()
}

func convertParaToRoleForInsert(form *ParaCreateRolePolicies) (role *models.Role, err error) {

	// 此版本暂不支持插入父角色
	role = models.NewRole(object.NewCollection(&object.HashMap{
		"name": form.Name,
		//"parentID": form.ParentID,
	}))

	err = role.CheckRoleNameAvailable(global.G_DBConnection)
	if err != nil {
		return nil, err
	}

	return role, err
}
