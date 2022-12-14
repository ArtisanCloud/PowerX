package service

import (
	"errors"
	database2 "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/contract"
	modelWecom "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/weCom"
	global2 "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
)

type DepartmentService struct {
	*Service
	Department *models.Department
}

func NewDepartmentService(ctx *gin.Context) (r *DepartmentService) {
	r = &DepartmentService{
		Service:    NewService(ctx),
		Department: models.NewDepartment(nil),
	}
	return r
}

func (srv *DepartmentService) SyncDepartments(departmentID int) (err error) {

	response, err := weCom.G_WeComEmployee.App.Department.List(departmentID)
	if err != nil {
		return err
	}

	if response.ErrCode != 0 {
		return errors.New(response.ErrMSG)
	}

	arrayDepartments := []*models.Department{}
	for _, wxDepartment := range response.Departments {
		department := &models.Department{
			WXDepartment: &wx.WXDepartment{
				ID:       wxDepartment.ID,
				Name:     wxDepartment.Name,
				NameEN:   wxDepartment.NameEN,
				ParentID: wxDepartment.ParentID,
				Order:    wxDepartment.Order,
			},
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

func (srv *DepartmentService) GetTreeDepartments(db *gorm.DB, conditions *map[string]interface{}, departmentID *int) (departments []*models.Department, err error) {
	departments = []*models.Department{}

	if conditions == nil {
		conditions = &map[string]interface{}{}
	}

	if departmentID == nil {
		*departmentID = 0
	}
	(*conditions)["parent_id"] = departmentID

	departments, err = srv.GetDepartments(db, departmentID)
	if err != nil {
		return nil, err
	}

	for _, department := range departments {
		department.SubDepartments, err = srv.GetTreeDepartments(db, nil, &department.ID)

		// load employees
		department.Employees, err = department.LoadEmployees(db, nil)
	}

	return departments, err

}

func (srv *DepartmentService) GetDepartments(db *gorm.DB, parentID *int) (departments []*models.Department, err error) {
	departments = []*models.Department{}

	db = db.Table(srv.Department.GetTableName(true))

	var conditions *map[string]interface{} = nil
	if parentID != nil {
		conditions = &map[string]interface{}{
			"parent_id": *parentID,
		}
	}

	if conditions != nil {
		db = db.Where(*conditions)
	}

	db = db.Find(&departments)
	err = db.Error

	return departments, err
}

func (srv *DepartmentService) GetDepartmentsByIDs(db *gorm.DB, arrayIDs []int) (departments []*wx.WXDepartment, err error) {
	departments = []*wx.WXDepartment{}

	if len(arrayIDs) > 0 {
		db = db.Table(srv.Department.GetTableName(true)).Where("id in (?)", arrayIDs).Find(&departments)
		err = db.Error
	}

	return departments, err
}

func (srv *DepartmentService) UpsertDepartments(db *gorm.DB, uniqueName string, departments []*models.Department) error {

	if len(departments) <= 0 {
		return nil
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(database2.GetModelFields(models.Department{})),
	}).Create(&departments)

	return result.Error
}

// --------------------------------------------------
// handle events from WeComApp party
// --------------------------------------------------

func (srv *DepartmentService) HandleDepartmentCreate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecom.EventPartyCreate{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Create Party", zap.Any("msg", msg))

	serviceDepartment := NewDepartmentService(context)
	departmentID, err := strconv.Atoi(msg.ID)
	if err != nil {
		return err
	}
	// ??????????????????????????????
	err = serviceDepartment.SyncDepartments(departmentID)
	if err != nil {
		return err
	}

	serviceEmployee := NewEmployeeService(context)
	// ???????????????????????????
	err = serviceEmployee.SyncEmployees(departmentID, 1)

	return err
}

func (srv *DepartmentService) HandleDepartmentUpdate(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecom.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Update Party", zap.Any("msg", msg))

	// ???????????????????????????

	// ????????????????????????????????????

	return err
}

func (srv *DepartmentService) HandleDepartmentDelete(context *gin.Context, event contract.EventInterface) (err error) {

	msg := &modelWecom.EventExternalUserAdd{}
	err = event.ReadMessage(msg)
	if err != nil {
		return err
	}

	logger.Logger.Info("Handle Delete Party", zap.Any("msg", msg))

	// ???????????????????????????

	// ???????????????

	return err
}
