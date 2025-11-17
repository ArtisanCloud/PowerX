package decay_guard

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidInput = errors.New("decay: invalid input")
	ErrTaskNotFound = errors.New("decay: task not found")
)

type Threshold struct {
	Category string  `json:"category" yaml:"category"`
	Severity string  `json:"severity" yaml:"severity"`
	MaxAge   float64 `json:"maxAgeHours" yaml:"maxAgeHours"`
	SLAHours float64 `json:"slaHours" yaml:"slaHours"`
	Reason   string  `json:"reason" yaml:"reason"`
}

type Options struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	MetricsWriter   *instrumentation.DecayMetricsWriter
	ThresholdsPath  string
	Dispatcher      TaskDispatcher
	Clock           func() time.Time
}

type Service struct {
	db         *gorm.DB
	inst       *instrumentation.Instrumentation
	metrics    *instrumentation.DecayMetricsWriter
	dispatcher TaskDispatcher
	thresholds []Threshold
	clock      func() time.Time
}

type TaskDispatcher interface {
	Dispatch(ctx context.Context, task *models.DecayTask) error
	Close(ctx context.Context, task *models.DecayTask) error
}

type noopDispatcher struct{}

func (noopDispatcher) Dispatch(context.Context, *models.DecayTask) error { return nil }
func (noopDispatcher) Close(context.Context, *models.DecayTask) error    { return nil }

func NewService(opts Options) *Service {
	if opts.DB == nil {
		panic("decay guard requires db")
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	thresholds := loadThresholds(opts.ThresholdsPath)
	d := opts.Dispatcher
	if d == nil {
		d = noopDispatcher{}
	}
	return &Service{
		db:         opts.DB,
		inst:       opts.Instrumentation,
		metrics:    opts.MetricsWriter,
		dispatcher: d,
		thresholds: thresholds,
		clock:      opts.Clock,
	}
}

func loadThresholds(path string) []Threshold {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var payload struct {
		Thresholds []Threshold `json:"thresholds" yaml:"thresholds"`
	}
	if err := yaml.Unmarshal(data, &payload); err != nil {
		if jsonErr := json.Unmarshal(data, &payload); jsonErr != nil {
			return nil
		}
	}
	return payload.Thresholds
}

func (s *Service) RunScan(ctx context.Context, spaceID uuid.UUID, detected int) ([]*models.DecayTask, error) {
	if spaceID == uuid.Nil || detected <= 0 {
		return nil, ErrInvalidInput
	}
	repoTask := repo.NewDecayTaskRepository(s.db)
	repoSpace := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := repoSpace.FindByUUID(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, gorm.ErrRecordNotFound
	}
	threshold := s.resolveThreshold(space)
	if threshold.Category == "" {
		threshold.Category = "coverage"
	}
	if threshold.Severity == "" {
		threshold.Severity = "medium"
	}
	detectedAt := s.clock().UTC()
	sla := detectedAt.Add(s.slaDuration(threshold.SLAHours))
	tasks := make([]*models.DecayTask, 0, detected)
	for i := 0; i < detected; i++ {
		task := &models.DecayTask{
			SpaceUUID:  spaceID,
			Category:   threshold.Category,
			Severity:   threshold.Severity,
			Status:     "open",
			DetectedAt: detectedAt,
			SLADueAt:   sla,
		}
		created, err := repoTask.Create(ctx, task)
		if err != nil {
			return nil, err
		}
		if err := s.withDispatcher(func(d TaskDispatcher) error { return d.Dispatch(ctx, created) }); err != nil {
			s.log(ctx).WarnF(ctx, "decay guard: dispatch task failed: %v", err)
		}
		tasks = append(tasks, created)
	}
	if len(tasks) == 0 {
		return tasks, nil
	}
	backlog := s.countBacklog(ctx, repoTask, spaceID)
	s.recordMetrics(ctx, instrumentation.DecayMetricsSnapshot{
		Detected: len(tasks),
		Backlog:  backlog,
	})
	s.emitAudit(ctx, space, "knowledge.decay.detected", map[string]any{
		"space_id":   space.UUID.String(),
		"tenant_id":  space.TenantID.String(),
		"task_count": len(tasks),
		"severity":   threshold.Severity,
		"category":   threshold.Category,
		"reason":     threshold.Reason,
	})
	return tasks, nil
}

func (s *Service) Restore(ctx context.Context, taskID uuid.UUID, notes string, falsePositive bool) (*models.DecayTask, error) {
	if taskID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	repoTask := repo.NewDecayTaskRepository(s.db)
	repoSpace := repo.NewKnowledgeSpaceRepository(s.db)
	task, err := repoTask.GetByUUID(ctx, taskID.String(), nil)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, ErrTaskNotFound
	}
	task.Status = "closed"
	now := s.clock()
	task.ResolvedAt = &now
	task.Resolution = notes
	task.FalsePositive = falsePositive
	if _, err := repoTask.Update(ctx, task); err != nil {
		return nil, err
	}
	fp := 0
	if falsePositive {
		fp = 1
	}
	if err := s.withDispatcher(func(d TaskDispatcher) error { return d.Close(ctx, task) }); err != nil {
		s.log(ctx).WarnF(ctx, "decay guard: close task failed: %v", err)
	}
	openTasks, _ := repoTask.ListOpenBySpace(ctx, task.SpaceUUID)
	s.recordMetrics(ctx, instrumentation.DecayMetricsSnapshot{
		FalsePositive:    fp,
		Backlog:          len(openTasks),
		AverageFillHours: s.fillHours(task, now),
	})
	space, _ := repoSpace.FindByUUID(ctx, task.SpaceUUID)
	s.emitAudit(ctx, space, "knowledge.decay.restore", map[string]any{
		"task_id":        task.UUID.String(),
		"false_positive": task.FalsePositive,
		"notes":          notes,
	})
	return task, nil
}

func (s *Service) ListOpen(ctx context.Context, spaceID uuid.UUID) ([]*models.DecayTask, error) {
	if spaceID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	repoTask := repo.NewDecayTaskRepository(s.db)
	return repoTask.ListOpenBySpace(ctx, spaceID)
}

func (s *Service) resolveThreshold(_ *models.KnowledgeSpace) Threshold {
	if len(s.thresholds) == 0 {
		return Threshold{Category: "coverage", Severity: "medium", SLAHours: 24 * 7}
	}
	return s.thresholds[0]
}

func (s *Service) slaDuration(hours float64) time.Duration {
	if hours <= 0 {
		return 7 * 24 * time.Hour
	}
	return time.Duration(float64(time.Hour) * hours)
}

func (s *Service) countBacklog(ctx context.Context, repoTask *repo.DecayTaskRepository, spaceID uuid.UUID) int {
	open, err := repoTask.ListOpenBySpace(ctx, spaceID)
	if err != nil {
		s.log(ctx).WarnF(ctx, "decay guard: backlog query failed: %v", err)
		return 0
	}
	return len(open)
}

func (s *Service) recordMetrics(ctx context.Context, snapshot instrumentation.DecayMetricsSnapshot) {
	if s.metrics == nil {
		return
	}
	if snapshot.RecordedAt.IsZero() {
		snapshot.RecordedAt = s.clock().UTC()
	}
	if err := s.metrics.Store(snapshot); err != nil {
		s.log(ctx).WarnF(ctx, "decay guard: write metrics failed: %v", err)
	}
}

func (s *Service) emitAudit(ctx context.Context, space *models.KnowledgeSpace, action string, payload map[string]any) {
	if s.inst == nil || space == nil {
		return
	}
	raw, _ := json.Marshal(payload)
	s.inst.Audit(ctx, &dbm.AuditEvent{
		OccurredAt:   s.clock(),
		Source:       "knowledge.decay",
		Operation:    action,
		ResourceType: "knowledge_space",
		ResourceID:   space.UUID.String(),
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         raw,
	})
}

func (s *Service) withDispatcher(fn func(TaskDispatcher) error) error {
	if s.dispatcher == nil || fn == nil {
		return nil
	}
	return fn(s.dispatcher)
}

func (s *Service) fillHours(task *models.DecayTask, now time.Time) float64 {
	if task == nil || task.DetectedAt.IsZero() {
		return 0
	}
	return now.Sub(task.DetectedAt).Hours()
}

func (s *Service) log(ctx context.Context) *pxlog.Logger {
	if s.inst != nil {
		return s.inst.Logger(ctx)
	}
	return pxlog.GetGlobalLogger().WithContext(ctx)
}
