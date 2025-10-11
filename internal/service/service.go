package service

// internal/service/service.go

import (
	"gorm.io/gorm"
)

type BaseService struct {
	DB *gorm.DB
}
