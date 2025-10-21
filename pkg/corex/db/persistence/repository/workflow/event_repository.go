package workflow

import (
	"context"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// EventRepository 负责写入 WorkflowEvent 表。
type EventRepository struct {
	*repository.BaseRepository[modelworkflow.WorkflowEvent]
	db *gorm.DB
}

// NewEventRepository 创建事件仓储。
func NewEventRepository(db *gorm.DB) *EventRepository {
	return &EventRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.WorkflowEvent](db),
		db:             db,
	}
}

// Record 新增工作流事件。
func (r *EventRepository) Record(ctx context.Context, evt *modelworkflow.WorkflowEvent) (*modelworkflow.WorkflowEvent, error) {
	if evt == nil {
		return nil, nil
	}
	return r.BaseRepository.Create(ctx, evt)
}
