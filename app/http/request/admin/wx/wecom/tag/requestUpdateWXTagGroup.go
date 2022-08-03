package tag

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/gin-gonic/gin"
)

type ParaUpdateWXTagGroup struct {
	GroupID   string `form:"groupID" json:"groupID" binding:"required"`
	GroupName string `form:"groupName" json:"groupName"`
	Order     int    `form:"order" json:"order"`
}

func ValidateUpdateWXTagGroup(context *gin.Context) {
	var form ParaUpdateWXTagGroup

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	wxTagGroup := convertParaToWXTagGroupForUpdate(&form)
	context.Set("wxTagGroup", wxTagGroup)
	context.Next()
}

func convertParaToWXTagGroupForUpdate(form *ParaUpdateWXTagGroup) (wxTagGroup *wx.WXTagGroup) {

	wxTagGroup = wx.NewWXTagGroup(object.NewCollection(&object.HashMap{
		"groupID":   form.GroupID,
		"groupName": form.GroupName,
		"order":     form.Order,
	}))

	return wxTagGroup
}
