package wecom

import (
	database2 "github.com/ArtisanCloud/PowerLibs/v2/database"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeComDepartmentService struct {
	Service    *WeComService
	Department *modelWX.WXDepartment
}

func NewWeComDepartmentService(ctx *gin.Context) (r *WeComDepartmentService) {
	r = &WeComDepartmentService{
		Service:    G_WeComEmployee,
		Department: modelWX.NewWXDepartment(nil),
	}
	return r
}

func (srv *WeComDepartmentService) UpsertDepartments(db *gorm.DB, uniqueName string, departments []*modelWX.WXDepartment) error {

	if len(departments) <= 0 {
		return nil
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(database2.GetModelFields(modelWX.WXDepartment{})),
	}).Create(&departments)

	return result.Error
}
