package organization

import "PowerX/internal/model"

type WeWorkEmployee struct {
	model.Model

	WeWorkUserId   string
	Name           string
	Position       string
	Mobile         string `gorm:"unique"`
	Gender         string
	Email          string `gorm:"unique"`
	BizMail        string
	Avatar         string
	ThumbAvatar    string
	Telephone      string
	Alias          string
	Address        string
	OpenUserid     string
	MainDepartment int64
	Status         int
	QrCode         string

	RefEmployeeId int64
}

type WeWorkDepartment struct {
	model.Model

	DepId    int64
	Name     string
	NameEn   string
	ParentId int
	Order    int

	RefDepartmentId int64
}
