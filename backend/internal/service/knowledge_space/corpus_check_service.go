package knowledge_space

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	workflow "github.com/ArtisanCloud/PowerX/internal/workflow/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
)

type CorpusCheckService struct {
	db       *gorm.DB
	pipeline workflow.CorpusCheckPipeline
	clock    func() time.Time
}

type CorpusCheckServiceOptions struct {
	DB       *gorm.DB
	Pipeline workflow.CorpusCheckPipeline
	Clock    func() time.Time
}

func NewCorpusCheckService(opts CorpusCheckServiceOptions) *CorpusCheckService {
	if opts.DB == nil {
		panic("corpus check service requires db")
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	return &CorpusCheckService{db: opts.DB, pipeline: opts.Pipeline, clock: opts.Clock}
}

// Start 创建 Job 记录并投递异步事件。
func (s *CorpusCheckService) Start(ctx context.Context, tenantUUID string, spaceUUID uuid.UUID, requestedBy string) (*models.CorpusCheckJob, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	requestedBy = strings.TrimSpace(requestedBy)
	if tenantUUID == "" || spaceUUID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	now := s.clock().UTC()

	jobs := repo.NewCorpusCheckJobRepository(s.db)
	job := &models.CorpusCheckJob{
		TenantUUID:       tenantUUID,
		SpaceUUID:        spaceUUID,
		Status:           models.CorpusCheckStatusPending,
		SampleJobUUIDs:   datatypes.JSON([]byte("[]")),
		Metrics:          datatypes.JSON([]byte("{}")),
		Recommendations:  datatypes.JSON([]byte("[]")),
		TraceID:          uuid.NewString(),
		ErrorReason:      "",
		StartedAt:        nil,
		CompletedAt:      nil,
	}
	job.UUID = uuid.New()
	job.CreatedAt = now
	job.UpdatedAt = now

	created, err := jobs.Create(ctx, job)
	if err != nil {
		return nil, err
	}

	if s.pipeline == nil {
		return created, nil
	}
	if _, err := s.pipeline.Schedule(ctx, workflow.CorpusCheckInput{
		JobUUID:     created.UUID,
		SpaceID:     spaceUUID,
		RequestedBy: requestedBy,
	}); err != nil {
		failAt := s.clock().UTC()
		created.Status = models.CorpusCheckStatusFailed
		created.ErrorReason = err.Error()
		created.CompletedAt = &failAt
		created.UpdatedAt = failAt
		_, _ = jobs.Update(ctx, created)
		return created, err
	}
	return created, nil
}

func (s *CorpusCheckService) Get(ctx context.Context, jobUUID uuid.UUID) (*models.CorpusCheckJob, error) {
	if jobUUID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return repo.NewCorpusCheckJobRepository(s.db).FindByUUID(ctx, jobUUID)
}
