package svc

import (
	"PowerX/internal/config"
	"PowerX/internal/middleware"
	"PowerX/internal/uc"
	"PowerX/pkg/pluginx"
	"github.com/zeromicro/go-zero/rest"
)

type ServiceContext struct {
	Config config.Config
	PowerX *uc.PowerXUseCase
	Custom *uc.CustomUseCase

	MPCustomerJWTAuth     rest.Middleware
	MPCustomerGet         rest.Middleware
	WebCustomerJWTAuth    rest.Middleware
	WebCustomerGet        rest.Middleware
	EmployeeJWTAuth       rest.Middleware
	EmployeeNoPermJWTAuth rest.Middleware

	Plugin *pluginx.Manager
}

func NewServiceContext(c config.Config, opts ...Option) *ServiceContext {
	powerx, _ := uc.NewPowerXUseCase(&c)
	custom, _ := uc.NewCustomUseCase(&c)

	svcCtx := ServiceContext{
		Config:                c,
		PowerX:                powerx,
		MPCustomerJWTAuth:     middleware.NewMPCustomerJWTAuthMiddleware(&c, powerx).Handle,
		MPCustomerGet:         middleware.NewMPCustomerGetMiddleware(&c, powerx).Handle,
		WebCustomerJWTAuth:    middleware.NewWebCustomerJWTAuthMiddleware(&c, powerx).Handle,
		WebCustomerGet:        middleware.NewWebCustomerGetMiddleware(&c, powerx).Handle,
		EmployeeJWTAuth:       middleware.NewEmployeeJWTAuthMiddleware(&c, powerx).Handle,
		EmployeeNoPermJWTAuth: middleware.NewEmployeeNoPermJWTAuthMiddleware(&c, powerx).Handle,
		Custom:                custom,
	}

	for _, opt := range opts {
		opt(&svcCtx)
	}

	return &svcCtx
}

type Option func(context *ServiceContext)

func WithPlugin(plugin *pluginx.Manager) Option {
	return func(context *ServiceContext) {
		context.Plugin = plugin
	}
}
