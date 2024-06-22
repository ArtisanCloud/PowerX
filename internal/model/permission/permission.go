package permission

import "PowerX/internal/model"

type AdminAuthMetadataKey struct{}

type AdminAuthMetadata struct {
	UID int64
}

type UserCasbinPolicy struct {
	ID    int64 `gorm:"primarykey"`
	PType string
	V0    string
	V1    string
	V2    string
	V3    string
	V4    string
	V5    string
}

type AdminAPI struct {
	model.CommonModel
	API     string
	Method  string
	Name    string
	Desc    string
	GroupId int64
	Group   AdminAPIGroup
}

type AdminAPIGroup struct {
	model.Model
	GroupCode string `gorm:"unique"`
	Prefix    string
	Name      string
	Desc      string
}

type AdminRole struct {
	model.Model
	RoleCode   string `gorm:"unique"`
	Name       string
	Desc       string
	IsReserved bool
	AdminAPI   []*AdminAPI `gorm:"many2many:admin_role_apis"`
	MenuNames  []*AdminRoleMenuName
}

type AdminRoleMenuName struct {
	model.CommonModel
	AdminRoleId int64
	MenuName    string
}
