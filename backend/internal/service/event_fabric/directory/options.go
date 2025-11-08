package directory

import (
	"context"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Clock 定义时间获取函数，便于测试。
type Clock func() time.Time

// ActorResolver 提供当前操作人信息。
type ActorResolver func(ctx context.Context) string

// TopicStore 定义主题存储依赖，可由测试或具体仓储实现。
type TopicStore interface {
	Create(ctx context.Context, topic *model.TopicDefinition) (*model.TopicDefinition, error)
	Update(ctx context.Context, topic *model.TopicDefinition) (*model.TopicDefinition, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*model.TopicDefinition, error)
	FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*model.TopicDefinition, error)
	List(ctx context.Context, query eventfabricrepo.QueryContext) ([]*model.TopicDefinition, int64, error)
}

// Options 构造目录服务的配置。
type Options struct {
	DB                *gorm.DB
	Store             TopicStore
	EventBus          event_bus.EventBus
	Clock             Clock
	ActorResolver     ActorResolver
	DefaultMaxRetry   int
	DefaultAckTimeout time.Duration
}
