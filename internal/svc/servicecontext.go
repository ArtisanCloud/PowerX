package svc

import (
	"PowerX/internal/config"
	"PowerX/internal/middleware"
	"PowerX/internal/uc"
	"github.com/zeromicro/go-zero/rest"
)

type ServiceContext struct {
	Config config.Config
	PowerX *uc.PowerXUseCase

	EmployeeJWTAuth rest.Middleware
}

func NewServiceContext(c config.Config) *ServiceContext {
	powerx, _ := uc.NewPowerXUseCase(&c)

	return &ServiceContext{
		Config:          c,
		PowerX:          powerx,
		EmployeeJWTAuth: middleware.NewEmployeeJWTAuthMiddleware(&c, powerx).Handle,
	}
}
