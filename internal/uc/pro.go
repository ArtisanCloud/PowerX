package uc

import (
	"PowerX/internal/config"
	"gorm.io/gorm"
)

type ProUseCase struct {
	db *gorm.DB
}

func NewProUseCase(conf *config.Config, pxUseCase *PowerXUseCase) (uc *ProUseCase, clean func()) {

	uc = &ProUseCase{}

	// 需要打印当时系统的Timezone
	return uc, func() {

	}
}
