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

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e Employee) Table() string {
    return `employees`
}
