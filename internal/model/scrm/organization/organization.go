package organization

import (
	"PowerX/internal/model"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

type Employee struct {
	model.Model
	Account       string `gorm:"unique"`
	Name          string
	NickName      string
	Desc          string
	Position      string
	JobTitle      string
	DepartmentId  int64
	Department    *Department
	MobilePhone   string
	Gender        string
	Email         string
	ExternalEmail string
	Avatar        string
	Password      string
	Status        string `gorm:"index"`
	IsReserved    bool
	IsActivated   bool
}

func (e *Employee) HashPassword() (err error) {
	if e.Password != "" {
		e.Password, err = HashPassword(e.Password)
	}
	return nil
}

const (
	GenderMale   = "male"
	GenderFeMale = "female"
	GenderUnKnow = "un_know"
)

const (
	EmployeeStatusDisabled = "disabled"
	EmployeeStatusEnabled  = "enabled"
)

const defaultCost = bcrypt.MinCost

// 生成哈希密码
func HashPassword(password string) (hashedPwd string, err error) {
	newPassword, err := bcrypt.GenerateFromPassword([]byte(password), defaultCost)
	if err != nil {
		return "", errors.Wrap(err, "gen pwd failed")
	}
	return string(newPassword), nil
}

// 校验密码
func VerifyPassword(hashedPwd string, pwd string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPwd), []byte(pwd))
	return err == nil
}

type Department struct {
	model.Model
	Name        string
	PId         int64
	PDep        *Department `gorm:"foreignKey:PId"`
	LeaderId    int64
	Leader      *Employee     `gorm:"foreignKey:LeaderId"`
	Ancestors   []*Department `gorm:"many2many:department_ancestors;"`
	Desc        string
	PhoneNumber string
	Email       string
	Remark      string
	IsReserved  bool
}

type WeWorkEmployee struct {
	model.Model

	WeWorkUserId           string `gorm:"unique"`
	Name                   string
	Position               string
	Mobile                 string `gorm:"unique"`
	Gender                 string
	Email                  string `gorm:"unique"`
	BizMail                string
	Avatar                 string
	ThumbAvatar            string
	Telephone              string
	Alias                  string
	Address                string
	OpenUserid             string
	WeWorkMainDepartmentId int
	Status                 int
	QrCode                 string

	RefEmployeeId int64
}

type WeWorkDepartment struct {
	model.Model

	WeWorkDepId    int `gorm:"unique"`
	Name           string
	NameEn         string
	WeWorkParentId int
	Order          int

	RefDepartmentId int64
}
