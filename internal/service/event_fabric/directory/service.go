package directory

import (
	"context"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
)

// Topic 描述目录层面的主题信息，供服务层与 Handler 共享。
type Topic struct {
	ID            string
	TenantID      string
	Namespace     string
	Name          string
	PayloadFormat string
	MaxRetry      int32
	Lifecycle     model.TopicLifecycle
	Version       int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CreateTopicInput 用于创建或导入新的主题。
type CreateTopicInput struct {
	TenantID      string
	Namespace     string
	Name          string
	PayloadFormat string
	MaxRetry      int32
}

// UpdateLifecycleInput 控制主题生命周期迁移。
type UpdateLifecycleInput struct {
	TopicID       string
	TargetState   model.TopicLifecycle
	DeprecatedAt  *time.Time
	RetiredAt     *time.Time
	ChangeReason  string
	OperatorID    string
	OperatorEmail string
}

// Service 定义主题目录的核心操作集合。
type Service interface {
	CreateTopic(ctx context.Context, input CreateTopicInput) (*Topic, error)
	UpdateLifecycle(ctx context.Context, input UpdateLifecycleInput) (*Topic, error)
	ListTopics(ctx context.Context, query repository.QueryContext) ([]*Topic, int64, error)
}
