package service

import (
	"errors"
	database2 "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/contract"
	models2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	global2 "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DepartmentService struct {
	*Service
	Department *wx.WXDepartment
}

func NewDepartmentService(ctx *gin.Context) (r *DepartmentService) {
	r = &DepartmentService{
		Service:    NewService(ctx),
		Department: wx.NewWXDepartment(nil),
	}
	return r
}

func (srv *DepartmentService) SyncDepartments() (err error) {

	response, err := wecom.G_WeComEmployee.App.Department.List(0)
	if err != nil {
		return err
	}

	if response.ErrCode != 0 {
		return errors.New(response.ErrMSG)
	}

	arrayDepartments := []*wx.WXDepartment{}
	for _, wxDepartment := range response.Departments {
		department := &wx.WXDepartment{
			ID:       wxDepartment.ID,
			Name:     wxDepartment.Name,
			NameEN:   wxDepartment.NameEN,
			ParentID: wxDepartment.ParentID,
			Order:    wxDepartment.Order,
		}

		arrayDepartments = append(arrayDepartments, department)
	}
	err = srv.UpsertDepartments(global.G_DBConnection, wx.DEPARTMENT_UNIQUE_ID, arrayDepartments)

	return err
}

func (srv *DepartmentService) GetDepartment(db *gorm.DB, departmentName string) (department *wx.WXDepartment, r int) {

	department = &wx.WXDepartment{}

	db = db.Scopes(
		srv.Department.WhereWXDepartmentName(departmentName),
		//srv.Department.WhereIsValid,
	)

	result := db.Preload("Account").First(department)

	if result.RowsAffected > 0 {
		//fmt.Printf("department: %v", department.Account)
		return department, global2.API_RESULT_CODE_INIT

	} else {
		return nil, global2.API_ERR_CODE_USER_UNREGISTER
	}

}

func (srv *DepartmentService) GetDepartments(db *gorm.DB) (departments []*wx.WXDepartment, err error) {
	departments = []*wx.WXDepartment{}

	db = db.Table(wx.TABLE_NAME_DEPARTMENT).Find(&departments)
	err = db.Error

	return departments, err
}

func (srv *DepartmentService) GetDepartmentsByIDs(db *gorm.DB, arrayIDs []int) (departments []*wx.WXDepartment, err error) {
	departments = []*wx.WXDepartment{}

	if len(arrayIDs) > 0 {
		db = db.Table(wx.TABLE_NAME_DEPARTMENT).Where("id in (?)", arrayIDs).Find(&departments)
		err = db.Error
	}

	return departments, err
}

func (srv *DepartmentService) UpsertDepartments(db *gorm.DB, uniqueName string, departments []*wx.WXDepartment) error {

	if len(departments) <= 0 {
		return nil
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(database2.GetModelFields(wx.WXDepartment{})),
	}).Create(&departments)

	return result.Error
}

// --------------------------------------------------
// handle events from WeComApp party
// --------------------------------------------------

func (srv *DepartmentService) HandleDepartmentCreate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &models2.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Create Party", zap.Any("msg", msg))

	return err
}

func (srv *DepartmentService) HandleDepartmentUpdate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &models2.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Update Party", zap.Any("msg", msg))

	return err
}

func (srv *DepartmentService) HandleDepartmentDelete(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &models2.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Delete Party", zap.Any("msg", msg))

	return err
}
