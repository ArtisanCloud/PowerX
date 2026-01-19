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
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
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

type RunScanInput struct {
	SpaceID     uuid.UUID
	Detected    int
	Category    string
	Severity    string
	Reason      string
	AssignedTo  string
	RequestedBy string
}

type RestoreInput struct {
	TaskID        uuid.UUID
	Notes         string
	FalsePositive bool
	ApprovedBy    string
	Reason        string
}

type Options struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	MetricsWriter   *instrumentation.DecayMetricsWriter
	ThresholdsPath  string
	Dispatcher      TaskDispatcher
	EventBus        event_bus.EventBus
	DispatchTopic   string
	CloseTopic      string
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
	if d == nil && opts.EventBus != nil {
		d = newEventBusDispatcher(opts.EventBus, opts.DispatchTopic, opts.CloseTopic)
	}
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
	return s.RunScanWithInput(ctx, RunScanInput{SpaceID: spaceID, Detected: detected})
}

func (s *Service) RunScanWithInput(ctx context.Context, input RunScanInput) ([]*models.DecayTask, error) {
	if input.SpaceID == uuid.Nil || input.Detected <= 0 {
		return nil, ErrInvalidInput
	}
	repoTask := repo.NewDecayTaskRepository(s.db)
	repoSpace := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := repoSpace.FindByUUID(ctx, input.SpaceID)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if !tenantMatch(ctx, space.TenantUUID) {
		return nil, gorm.ErrRecordNotFound
	}
	threshold := s.resolveThreshold(input.Category, input.Severity)
	if threshold.Category == "" {
		threshold.Category = "coverage"
	}
	if threshold.Severity == "" {
		threshold.Severity = "medium"
	}
	detectedAt := s.clock().UTC()
	sla := detectedAt.Add(s.slaDuration(threshold.SLAHours))
	tasks := make([]*models.DecayTask, 0, input.Detected)
	for i := 0; i < input.Detected; i++ {
		task := &models.DecayTask{
			SpaceUUID:  input.SpaceID,
			Category:   threshold.Category,
			Severity:   threshold.Severity,
			Status:     "open",
			DetectedAt: detectedAt,
			SLADueAt:   sla,
			AssignedTo: strings.TrimSpace(input.AssignedTo),
		}
		if strings.TrimSpace(input.Reason) != "" {
			task.Resolution = strings.TrimSpace(input.Reason)
		} else if strings.TrimSpace(threshold.Reason) != "" {
			task.Resolution = strings.TrimSpace(threshold.Reason)
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
	backlog := s.countBacklog(ctx, repoTask, input.SpaceID)
	s.recordMetrics(ctx, instrumentation.DecayMetricsSnapshot{
		Detected: len(tasks),
		Backlog:  backlog,
	})
	s.emitAudit(ctx, space, "knowledge.decay.detected", map[string]any{
		"space_id":    space.UUID.String(),
		"tenant_uuid": space.TenantUUID,
		"task_count":  len(tasks),
		"severity":    threshold.Severity,
		"category":    threshold.Category,
		"reason":      firstNonEmpty(input.Reason, threshold.Reason),
		"assigned_to": strings.TrimSpace(input.AssignedTo),
		"requestedBy": strings.TrimSpace(input.RequestedBy),
	})
	return tasks, nil
}

func (s *Service) Restore(ctx context.Context, taskID uuid.UUID, notes string, falsePositive bool) (*models.DecayTask, error) {
	return s.RestoreWithInput(ctx, RestoreInput{
		TaskID:        taskID,
		Notes:         notes,
		FalsePositive: falsePositive,
	})
}

func (s *Service) RestoreWithInput(ctx context.Context, input RestoreInput) (*models.DecayTask, error) {
	if input.TaskID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	repoTask := repo.NewDecayTaskRepository(s.db)
	repoSpace := repo.NewKnowledgeSpaceRepository(s.db)
	task, err := repoTask.GetByUUID(ctx, input.TaskID.String(), nil)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, ErrTaskNotFound
	}
	space, err := repoSpace.FindByUUID(ctx, task.SpaceUUID)
	if err != nil {
		return nil, err
	}
	if space == nil || !tenantMatch(ctx, space.TenantUUID) {
		return nil, ErrTaskNotFound
	}

	approvedBy := strings.TrimSpace(input.ApprovedBy)
	if approvedBy == "" {
		approvedBy = strings.TrimSpace(reqctx.GetSubject(ctx))
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = strings.TrimSpace(input.Notes)
	}
	task.Status = "closed"
	now := s.clock()
	task.ResolvedAt = &now
	task.Resolution = strings.TrimSpace(input.Notes)
	task.FalsePositive = input.FalsePositive
	updates := map[string]any{
		"status":         task.Status,
		"resolved_at":    task.ResolvedAt,
		"resolution":     task.Resolution,
		"false_positive": task.FalsePositive,
	}
	if _, err := repoTask.Patch(ctx, map[string]any{"uuid": task.UUID.String()}, updates); err != nil {
		return nil, err
	}
	fp := 0
	if task.FalsePositive {
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
	s.emitAudit(ctx, space, "knowledge.decay.restore", map[string]any{
		"task_id":        task.UUID.String(),
		"false_positive": task.FalsePositive,
		"notes":          strings.TrimSpace(input.Notes),
		"approved_by":    approvedBy,
		"reason":         reason,
	})
	return task, nil
}

func (s *Service) ListOpen(ctx context.Context, spaceID uuid.UUID) ([]*models.DecayTask, error) {
	if spaceID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	repoSpace := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := repoSpace.FindByUUID(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	if space == nil || !tenantMatch(ctx, space.TenantUUID) {
		return []*models.DecayTask{}, nil
	}
	repoTask := repo.NewDecayTaskRepository(s.db)
	return repoTask.ListOpenBySpace(ctx, spaceID)
}

func (s *Service) ListOpenByTenant(ctx context.Context, tenantUUID uuid.UUID, severity string) ([]*models.DecayTask, error) {
	if tenantUUID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if !tenantMatch(ctx, tenantUUID.String()) {
		return []*models.DecayTask{}, nil
	}
	repoTask := repo.NewDecayTaskRepository(s.db)
	return repoTask.ListOpenByTenant(ctx, tenantUUID.String(), severity)
}

func (s *Service) resolveThreshold(category, severity string) Threshold {
	if len(s.thresholds) == 0 {
		return Threshold{Category: "coverage", Severity: "medium", SLAHours: 24 * 7}
	}
	category = strings.TrimSpace(category)
	severity = strings.TrimSpace(severity)
	for _, th := range s.thresholds {
		if category != "" && strings.EqualFold(th.Category, category) {
			return th
		}
	}
	for _, th := range s.thresholds {
		if severity != "" && strings.EqualFold(th.Severity, severity) {
			return th
		}
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
	snapshot.EnsureMetrics()
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
		TenantUUID:   space.TenantUUID,
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

func tenantMatch(ctx context.Context, tenantUUID string) bool {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" {
		return true
	}
	scoped := strings.ToLower(strings.TrimSpace(reqctx.GetTenantUUID(ctx)))
	if scoped == "" {
		return true
	}
	return scoped == tenantUUID
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
