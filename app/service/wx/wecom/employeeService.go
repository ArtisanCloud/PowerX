package wecom

import (
	"errors"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerSocialite/v2/src/providers"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeComEmployeeService struct {
	WeComService *WeComService
	Employee     *models.Employee
}

func NewWeComEmployeeService(ctx *gin.Context) (r *WeComEmployeeService) {
	weComConfig, _ := object.StructToMap(config.AppConfigure.Wechat["wecom"])
	if weComConfig["contact_secret"] != nil {
		weComConfig["secret"] = weComConfig["contact_secret"]
	}
	r = &WeComEmployeeService{
		WeComService: G_WeComEmployee,
		Employee:     models.NewEmployee(nil),
	}
	return r
}

func (srv *WeComEmployeeService) UpsertEmployeeByWXEmployee(db *gorm.DB, employee *wx.WXEmployee) (err error) {
	err = srv.UpsertEmployees(db, models.EMPLOYEE_UNIQUE_ID, []*models.Employee{
		&models.Employee{
			PowerModel: databasePowerLib.NewPowerModel(),
			WXEmployee: &wx.WXEmployee{
				WXAlias:           employee.WXAlias,
				WXAvatar:          employee.WXAvatar,
				WXDepartments:     employee.WXDepartments,
				WXEmail:           employee.WXEmail,
				WXEnable:          employee.WXEnable,
				WXEnglishName:     employee.WXEnglishName,
				WXExtAttr:         employee.WXExtAttr,
				WXExternalProfile: employee.WXExternalProfile,
				WXGender:          employee.WXGender,
				WXHideMobile:      employee.WXHideMobile,
				WXIsLeader:        employee.WXIsLeader,
				WXIsLeaderInDept:  employee.WXIsLeaderInDept,
				WXMainDepartment:  employee.WXMainDepartment,
				WXMobile:          employee.WXMobile,
				WXName:            employee.WXName,
				WXOrder:           employee.WXOrder,
				WXPosition:        employee.WXPosition,
				WXQrCode:          employee.WXQrCode,
				WXStatus:          employee.WXStatus,
				WXTelephone:       employee.WXTelephone,
				WXThumbAvatar:     employee.WXThumbAvatar,
				WXCorpID:          employee.WXCorpID,
				WXOpenUserID:      employee.WXOpenUserID,
				WXUserID:          employee.WXUserID,
				WXOpenID:          employee.WXOpenID,
			},
		},
	}, []string{
		"updated_at",
		"wx_alias",
		"wx_avatar",
		"wx_department",
		"wx_email",
		"wx_enable",
		"wx_englishName",
		"wx_extAttr",
		"wx_externalProfile",
		"wx_gender",
		"wx_hideMobile",
		"wx_isLeader",
		"wx_isLeaderInDept",
		"wx_mainDepartment",
		"wx_mobile",
		"wx_name",
		"wx_order",
		"wx_position",
		"wx_qrCode",
		"wx_status",
		"wx_telephone",
		"wx_thumbAvatar",
		"wx_corp_id",
		"wx_open_user_id",
		"wx_open_id",
	})

	return err
}

func (srv *WeComEmployeeService) UpsertEmployees(db *gorm.DB, uniqueName string, employees []*models.Employee, fieldsToUpdate []string) error {

	if len(employees) <= 0 {
		return nil
	}

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = databasePowerLib.GetModelFields(&models.Employee{})
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(fieldsToUpdate),
	}).Create(&employees)

	return result.Error
}

func (srv *WeComEmployeeService) GetEmployees(db *gorm.DB) (employees []*models.Employee, err error) {
	employees = []*models.Employee{}

	db = db.Table(srv.Employee.GetTableName(true))

	result := db.Find(&employees)
	err = result.Error

	return employees, nil

}

func (srv *WeComEmployeeService) GetEmployeeByUserID(db *gorm.DB, userID string) (employee *models.Employee, err error) {
	employee = &models.Employee{}

	condition := &map[string]interface{}{
		"wx_user_id": userID,
	}
	err = databasePowerLib.GetFirst(db, condition, employee, nil)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	return employee, err
}

func GetMockWXUser() (user *providers.User) {

	user = providers.NewUser(&object.HashMap{
		"alias":        "",
		"avatar":       "http://wework.qpic.cn/bizmail/RdOJmwNrQZ86w2x45icOjYWg2PhwC5DSQWH2N8A2aKickeaTgeC9iciaJA/0",
		"department":   []int{1, 5, 6},
		"email":        "",
		"enable":       1,
		"english_name": "",
		"errcode":      0,
		"errmsg":       "ok",
		"extAttr": &object.HashMap{
			"attrs": &object.HashMap{},
		},
		"gender":         "1",
		"hideMobile":     0,
		"isLeaderInDept": []int{},
		"isLeader":       0,
		"mainDepartment": 0,
		"mobile":         "17721110156",
		"name":           "Michael Hu",
		"order":          []int{},
		"position":       "",
		"contactWay":     "https://open.work.weixin.qq.com/wwopen/userContactWay?vcode=vcc01ed468c0ccdd15",
		"status":         3,
		"telephone":      "",
		"thumbAvatar":    "http://wework.qpic.cn/bizmail/RdOJmwNrQZ86w2x45icOjYWg2PhwC5DSQWH2N8A2aKickeaTgeC9iciaJA/100",
		"userID":         "michaelhu",
	}, nil)

	return user
}
