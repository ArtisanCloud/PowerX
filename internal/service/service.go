package service

import (
	"gorm.io/gorm"
)

type BaseService struct {
	DB *gorm.DB
}
