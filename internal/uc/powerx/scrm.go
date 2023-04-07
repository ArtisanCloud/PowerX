package powerx

import "gorm.io/gorm"

type SCRMUseCase struct {
	db *gorm.DB
}

func NewSCRMUseCase(db *gorm.DB) *SCRMUseCase {
	return &SCRMUseCase{db: db}
}
