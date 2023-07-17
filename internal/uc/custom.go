package uc

import (
    "PowerX/internal/config"
    "gorm.io/gorm"
)

type CustomUseCase struct {
    db *gorm.DB
}

func NewCustomUseCase(conf *config.Config) (uc *CustomUseCase, clean func()) {

    return uc, func() {

    }
}
