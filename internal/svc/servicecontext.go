package svc

import (
	"PowerX/internal/config"
	"PowerX/internal/uc"
)

type ServiceContext struct {
	Config config.Config
	PowerX *uc.PowerXUseCase
}

func NewServiceContext(c config.Config) *ServiceContext {
	powerx, _ := uc.NewPowerXUseCase(&c)

	return &ServiceContext{
		Config: c,
		PowerX: powerx,
	}
}
