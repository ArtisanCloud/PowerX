package middleware

// TenantHeaderPolicy 控制租户头部策略。
type TenantHeaderPolicy struct {
	RequireUUID bool
}

type jwtMiddlewareConfig struct {
	headerPolicy TenantHeaderPolicy
}

// JwtOption 自定义中间件行为。
type JwtOption func(*jwtMiddlewareConfig)

// WithTenantHeaderPolicy 配置头部策略。
func WithTenantHeaderPolicy(policy TenantHeaderPolicy) JwtOption {
	return func(c *jwtMiddlewareConfig) {
		c.headerPolicy = policy
	}
}
