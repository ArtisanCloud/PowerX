package organization

import "PowerX/internal/model"

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
