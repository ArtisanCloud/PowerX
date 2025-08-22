package service

import (
	"context"
	"gorm.io/gorm"
)

type BaseService struct {
	DB *gorm.DB
}

func (s *BaseService) TX(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.DB.WithContext(ctx).Transaction(fn)
}
