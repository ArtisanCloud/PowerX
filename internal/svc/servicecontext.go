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
	Custom *uc.CustomUseCase

	MPCustomerJWTAuth     rest.Middleware
	MPCustomerGet         rest.Middleware
	EmployeeJWTAuth       rest.Middleware
	EmployeeNoPermJWTAuth rest.Middleware
}

func NewServiceContext(c config.Config) *ServiceContext {
	powerx, _ := uc.NewPowerXUseCase(&c)
	custom, _ := uc.NewCustomUseCase(&c)

	return &ServiceContext{
		Config:                c,
		PowerX:                powerx,
		MPCustomerJWTAuth:     middleware.NewMPCustomerJWTAuthMiddleware(&c, powerx).Handle,
		MPCustomerGet:         middleware.NewMPCustomerGetMiddleware(&c, powerx).Handle,
		EmployeeJWTAuth:       middleware.NewEmployeeJWTAuthMiddleware(&c, powerx).Handle,
		EmployeeNoPermJWTAuth: middleware.NewEmployeeNoPermJWTAuthMiddleware(&c, powerx).Handle,
		Custom:                custom,
	}
}
