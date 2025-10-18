package shared

// internal/app/shared/options.go

import (
	"github.com/ArtisanCloud/PowerX/internal/service/auth"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
)

type DepsOptions struct {
	AuthUser     auth.AuthOptions      // 给用户端的 Audience
	AuthCustomer auth.AuthOptions      // 给客户/插件端的 Audience
	Audit        auditsvc.AuditOptions // 批量大小、等待等
	Storage      mediasvc.StorageOptions
	// 以后需要别的也放在这里（如默认租户、开关等）
	EventFabric EventFabricOptions
}

// EventFabricOptions 描述事件骨干依赖的运行配置。
type EventFabricOptions struct {
	AckTimeoutSeconds int
	DefaultMaxRetry   int
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	RetryKeyPrefix    string
	ReplayKeyPrefix   string
	SchedulerInterval int
}
