package uc

import (
	"PowerX/internal/config"
	"gorm.io/gorm"
)

const AuthPlatformKey = "AuthPlatform"
const AuthPlatformId = "PlatformId"

type OpenAPIUseCase struct {
	db *gorm.DB
}

func NewOpenAPIUseCase(conf *config.Config, pxUseCase *PowerXUseCase) (uc *OpenAPIUseCase, clean func()) {

	uc = &OpenAPIUseCase{}
	return uc, func() {}
}
