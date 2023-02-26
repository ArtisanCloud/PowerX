package svc

import (
	"PowerX/internal/config"
	"PowerX/internal/uc"
)

type ServiceContext struct {
	Config config.Config
	UC     *uc.PowerXUseCase
}

func NewServiceContext(c config.Config) *ServiceContext {
	uc, _ := uc.NewPowerXUseCase(&c)

	return &ServiceContext{
		Config: c,
		UC:     uc,
	}
}
