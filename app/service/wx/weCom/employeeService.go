package weCom

import (
	"errors"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	models2 "github.com/ArtisanCloud/PowerSocialite/v2/src/models"
	"github.com/ArtisanCloud/PowerSocialite/v2/src/providers"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WeComEmployeeService struct {
	WeComService *WeComService
	Employee     *models.Employee
}

func NewWeComEmployeeService(ctx *gin.Context) (r *WeComEmployeeService) {
	r = &WeComEmployeeService{
		WeComService: G_WeComEmployee,
		Employee:     models.NewEmployee(nil),
	}
	return r
}

func (srv *WeComEmployeeService) UpsertEmployeeByWXEmployee(db *gorm.DB, employee *models.Employee) (err error) {

	employee.PowerModel = databasePowerLib.NewPowerModel()
	employee.WXEmployee = &wx.WXEmployee{
		WXAlias:           employee.WXEmployee.WXAlias,
		WXAvatar:          employee.WXEmployee.WXAvatar,
		WXDepartment:      employee.WXEmployee.WXDepartment,
		WXEmail:           employee.WXEmployee.WXEmail,
		WXEnable:          employee.WXEmployee.WXEnable,
		WXEnglishName:     employee.WXEmployee.WXEnglishName,
		WXExtAttr:         employee.WXEmployee.WXExtAttr,
		WXExternalProfile: employee.WXEmployee.WXExternalProfile,
		WXGender:          employee.WXEmployee.WXGender,
		WXHideMobile:      employee.WXEmployee.WXHideMobile,
		WXIsLeader:        employee.WXEmployee.WXIsLeader,
		WXIsLeaderInDept:  employee.WXEmployee.WXIsLeaderInDept,
		WXMainDepartment:  employee.WXEmployee.WXMainDepartment,
		WXMobile:          employee.WXEmployee.WXMobile,
		WXName:            employee.WXEmployee.WXName,
		WXOrder:           employee.WXEmployee.WXOrder,
		WXPosition:        employee.WXEmployee.WXPosition,
		WXQrCode:          employee.WXEmployee.WXQrCode,
		WXStatus:          employee.WXEmployee.WXStatus,
		WXTelephone:       employee.WXEmployee.WXTelephone,
		WXThumbAvatar:     employee.WXEmployee.WXThumbAvatar,
		WXCorpID:          employee.WXEmployee.WXCorpID,
		WXOpenUserID:      employee.WXEmployee.WXOpenUserID,
		WXUserID:          employee.WXEmployee.WXUserID,
		WXOpenID:          employee.WXEmployee.WXOpenID,
	}
	err = srv.UpsertEmployees(db, []*models.Employee{
		employee,
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

func (srv *WeComEmployeeService) UpsertEmployees(db *gorm.DB, employees []*models.Employee, fieldsToUpdate []string) error {

	return databasePowerLib.UpsertModelsOnUniqueID(db, &models.Employee{}, models.EMPLOYEE_UNIQUE_ID, employees, fieldsToUpdate)
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
	preload := []string{"Role"}
	err = databasePowerLib.GetFirst(db, condition, employee, preload)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	return employee, err
}

func (srv *WeComEmployeeService) IsActive(employee *models.Employee) bool {
	return employee.WXEmployee.WXStatus == models2.EMPLOYEE_STATUS_ACTIVE
}

func (srv *WeComEmployeeService) IsProhibited(employee *models.Employee) bool {
	return employee.WXEmployee.WXStatus == models2.EMPLOYEE_STATUS_PROHIBITED
}

func (srv *WeComEmployeeService) IsInActive(employee *models.Employee) bool {
	return employee.WXEmployee.WXStatus == models2.EMPLOYEE_STATUS_INACTIVE
}

func (srv *WeComEmployeeService) IsQuit(employee *models.Employee) bool {
	return employee.WXEmployee.WXStatus == models2.EMPLOYEE_STATUS_QUIT
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
