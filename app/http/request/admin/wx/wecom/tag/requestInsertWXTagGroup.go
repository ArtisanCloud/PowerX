package tag

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/gin-gonic/gin"
)

type ParaInsertWXTagGroup struct {
	GroupID        string      `form:"groupID" json:"groupID"`
	GroupName      string      `form:"groupName" json:"groupName"`
	Order          int         `form:"order" json:"order"`
	WXDepartmentID int         `form:"wXDepartmentID" json:"wXDepartmentID"`
	Tags           []*wx.WXTag `form:"tags" json:"tags"`
}

func ValidateInsertWXTagGroup(context *gin.Context) {
	var form ParaInsertWXTagGroup

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	wxTagGroup := convertParaToWXTagGroupForUpsert(&form)
	context.Set("wxTagGroup", wxTagGroup)
	context.Next()
}

func convertParaToWXTagGroupForUpsert(form *ParaInsertWXTagGroup) (wxTagGroup *wx.WXTagGroup) {

	wxTagGroup = wx.NewWXTagGroup(object.NewCollection(&object.HashMap{
		"groupID":        form.GroupID,
		"groupName":      form.GroupName,
		"order":          form.Order,
		"wxDepartmentID": form.WXDepartmentID,
		"tags":           form.Tags,
	}))

	return wxTagGroup
}
