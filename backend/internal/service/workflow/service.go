package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/service"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// ErrNotImplemented 表示后续阶段才会补全的业务逻辑。
var ErrNotImplemented = errors.New("workflow service: not implemented")

// DefinitionStore 定义层持久化接口。
type DefinitionStore interface {
	CreateDefinition(ctx context.Context, def *modelworkflow.WorkflowDefinition) (*modelworkflow.WorkflowDefinition, error)
	NextVersion(ctx context.Context, tenantUUID string, name string) (int32, error)
	GetByUUID(ctx context.Context, tenantUUID string, definitionUUID uuid.UUID, version *int32) (*modelworkflow.WorkflowDefinition, error)
	GetLatestPublished(ctx context.Context, tenantUUID string, definitionUUID uuid.UUID) (*modelworkflow.WorkflowDefinition, error)
	ListByTenant(ctx context.Context, tenantUUID string, status []string, keyword string, limit, offset int) ([]modelworkflow.WorkflowDefinition, int64, error)
	UpdateStatus(ctx context.Context, tenantUUID string, definitionUUID uuid.UUID, version int32, status string, updates map[string]interface{}) error
}

// InstanceStore 实例层持久化接口。
type InstanceStore interface {
	CreateInstance(ctx context.Context, instance *modelworkflow.WorkflowInstance) (*modelworkflow.WorkflowInstance, error)
	GetByUUID(ctx context.Context, tenantUUID string, instanceUUID uuid.UUID) (*modelworkflow.WorkflowInstance, error)
	ListInstances(ctx context.Context, filter workflowrepo.InstanceListFilter) ([]modelworkflow.WorkflowInstance, int64, error)
	UpdateState(ctx context.Context, tenantUUID string, instanceUUID uuid.UUID, nextState string, updates map[string]interface{}) error
}

// StepRecordStore 步骤记录持久化接口。
type StepRecordStore interface {
	AppendRecord(ctx context.Context, record *modelworkflow.WorkflowStepRecord) (*modelworkflow.WorkflowStepRecord, error)
	GetByID(ctx context.Context, id uint64) (*modelworkflow.WorkflowStepRecord, error)
	ListByInstance(ctx context.Context, instanceUUID uuid.UUID) ([]modelworkflow.WorkflowStepRecord, error)
	FindLatestByStep(ctx context.Context, instanceUUID uuid.UUID, stepID string) (*modelworkflow.WorkflowStepRecord, error)
	UpdateState(ctx context.Context, id uint64, nextState string, updates map[string]interface{}) error
}

// AssignmentStore Agent 派发持久化接口。
type AssignmentStore interface {
	CreateAssignment(ctx context.Context, assignment *modelworkflow.AgentAssignment) (*modelworkflow.AgentAssignment, error)
	GetLatestByStep(ctx context.Context, stepRecordID uint64) (*modelworkflow.AgentAssignment, error)
	FindOpenAssignments(ctx context.Context, agentUUID uuid.UUID, statuses []string, limit int) ([]modelworkflow.AgentAssignment, error)
	UpdateStatus(ctx context.Context, id uint64, status string, updates map[string]interface{}) error
	FindTimedOutAssignments(ctx context.Context, tenantUUID string, before time.Time, limit int) ([]modelworkflow.AgentAssignment, error)
}

// CompensationStore 补偿记录持久化接口。
type CompensationStore interface {
	CreateCompensation(ctx context.Context, compensation *modelworkflow.WorkflowStepCompensation) (*modelworkflow.WorkflowStepCompensation, error)
	ListByInstance(ctx context.Context, instanceUUID uuid.UUID) ([]modelworkflow.WorkflowStepCompensation, error)
	UpdateState(ctx context.Context, id uint64, state string, updates map[string]interface{}) error
}

// EventRecorder 记录工作流事件。
type EventRecorder interface {
	RecordEvent(ctx context.Context, evt *modelworkflow.WorkflowEvent) error
}

// Service 聚合工作流定义、实例与调度相关逻辑。
type Service struct {
	*service.BaseService

	definitions   DefinitionStore
	instances     InstanceStore
	steps         StepRecordStore
	assignments   AssignmentStore
	compensations CompensationStore
	events        EventRecorder
	scheduler     *Scheduler
	tracker       *AssignmentTracker
	metrics       MetricsRecorder

	now func() time.Time
	em  *eventEmitter
}

// ServiceOptions 用于注入自定义依赖。
type ServiceOptions struct {
	DefinitionStore      DefinitionStore
	InstanceStore        InstanceStore
	StepStore            StepRecordStore
	AssignmentStore      AssignmentStore
	CompensationStore    CompensationStore
	EventRecorder        EventRecorder
	Clock                func() time.Time
	Scheduler            *Scheduler
	ReliableQueue        eventbus.ReliableQueue
	AssignmentTracker    *AssignmentTracker
	AssignmentAckTimeout time.Duration
	GrantValidator       ToolGrantValidator
	Metrics              MetricsRecorder
}

// NewService 构建工作流服务实例。
func NewService(db *gorm.DB, opts ServiceOptions) *Service {
	defStore := opts.DefinitionStore
	if defStore == nil {
		defStore = workflowrepo.NewDefinitionRepository(db)
	}

	instStore := opts.InstanceStore
	if instStore == nil {
		instStore = workflowrepo.NewInstanceRepository(db)
	}

	stepStore := opts.StepStore
	if stepStore == nil {
		stepStore = workflowrepo.NewStepRecordRepository(db)
	}

	assignStore := opts.AssignmentStore
	if assignStore == nil {
		assignStore = workflowrepo.NewAgentAssignmentRepository(db)
	}

	compStore := opts.CompensationStore
	if compStore == nil {
		compStore = workflowrepo.NewCompensationRepository(db)
	}

	eventStore := opts.EventRecorder
	if eventStore == nil {
		eventRepo := workflowrepo.NewEventRepository(db)
		eventStore = &eventRepositoryAdapter{repo: eventRepo}
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	metrics := opts.Metrics
	if metrics == nil {
		metrics = noopMetricsRecorder{}
	}

	scheduler := opts.Scheduler
	if scheduler == nil && opts.ReliableQueue != nil {
		scheduler = NewScheduler(opts.ReliableQueue)
	}

	tracker := opts.AssignmentTracker
	if tracker == nil {
		ackTimeout := opts.AssignmentAckTimeout
		if ackTimeout <= 0 {
			ackTimeout = 2 * time.Minute
		}
		tracker = NewAssignmentTracker(AssignmentTrackerOptions{
			AssignmentStore: assignStore,
			StepStore:       stepStore,
			GrantValidator:  opts.GrantValidator,
			Clock:           clock,
			AckTimeout:      ackTimeout,
		})
	}

	return &Service{
		BaseService:   &service.BaseService{DB: db},
		definitions:   defStore,
		instances:     instStore,
		steps:         stepStore,
		assignments:   assignStore,
		compensations: compStore,
		events:        eventStore,
		scheduler:     scheduler,
		tracker:       tracker,
		metrics:       metrics,
		now:           clock,
		em:            newEventEmitter(eventStore, clock),
	}
}

// CreateDefinitionInput 定义创建工作流所需参数。
type CreateDefinitionInput struct {
	TenantUUID         string
	Name               string
	Description        string
	CreatedBy          uuid.UUID
	Steps              []StepDefinition
	DefaultRetryPolicy map[string]any
	CompensationPolicy map[string]any
	SlaPolicy          map[string]any
	Metadata           map[string]any
}

// PublishDefinitionInput 定义发布工作流所需参数。
type PublishDefinitionInput struct {
	TenantUUID     string
	DefinitionUUID uuid.UUID
	Version        int32
	PublishedBy    uuid.UUID
	ChangeNote     string
}

// CreateDefinition 创建工作流定义（后续阶段补全具体逻辑）。
func (s *Service) CreateDefinition(ctx context.Context, input CreateDefinitionInput) (*modelworkflow.WorkflowDefinition, error) {
	if s == nil {
		return nil, errors.New("workflow service unavailable")
	}
	tenantUUID, err := normalizeTenantUUID(input.TenantUUID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("name is required")
	}
	if input.CreatedBy == uuid.Nil {
		return nil, errors.New("created_by is required")
	}

	result, err := ValidateStepDefinitions(input.Steps)
	if err != nil {
		return nil, err
	}

	version, err := s.definitions.NextVersion(ctx, tenantUUID, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve next version: %w", err)
	}

	stepJSON, err := json.Marshal(result.Steps)
	if err != nil {
		return nil, fmt.Errorf("marshal step graph failed: %w", err)
	}

	def := &modelworkflow.WorkflowDefinition{
		TenantUUID:           tenantUUID,
		Name:                 strings.TrimSpace(input.Name),
		Description:          strings.TrimSpace(input.Description),
		Version:              version,
		Status:               "draft",
		StepGraph:            datatypes.JSON(stepJSON),
		DefaultRetryPolicy:   toJSONOrEmpty(input.DefaultRetryPolicy),
		CompensationPolicy:   toJSONOrEmpty(input.CompensationPolicy),
		SlaPolicy:            toJSONOrEmpty(input.SlaPolicy),
		Metadata:             toJSONOrEmpty(input.Metadata),
		CreatedBy:            input.CreatedBy,
		InitialContextSchema: datatypes.JSON([]byte(`{}`)),
	}

	created, err := s.definitions.CreateDefinition(ctx, def)
	if err != nil {
		return nil, err
	}

	s.em.emit(ctx, newWorkflowEvent(
		created.TenantUUID,
		uuid.Nil,
		"workflow.definition.created",
		fmt.Sprintf("workflow %s@v%d created", created.Name, created.Version),
		map[string]any{"definition_uuid": created.UUID.String()},
	))

	return created, nil
}

// PublishDefinition 发布指定版本的工作流定义。
func (s *Service) PublishDefinition(ctx context.Context, input PublishDefinitionInput) (*modelworkflow.WorkflowDefinition, error) {
	if s == nil {
		return nil, errors.New("workflow service unavailable")
	}
	tenantUUID, err := normalizeTenantUUID(input.TenantUUID)
	if err != nil {
		return nil, err
	}
	if input.DefinitionUUID == uuid.Nil {
		return nil, errors.New("definition_id is required")
	}
	if input.PublishedBy == uuid.Nil {
		return nil, errors.New("published_by is required")
	}

	var versionPtr *int32
	if input.Version > 0 {
		versionPtr = &input.Version
	}

	definition, err := s.definitions.GetByUUID(ctx, tenantUUID, input.DefinitionUUID, versionPtr)
	if err != nil {
		return nil, err
	}
	if definition.Status == "published" {
		return definition, nil
	}

	now := s.now().UTC()
	update := map[string]interface{}{
		"published_at":      now,
		"last_published_by": input.PublishedBy,
		"last_change_note":  strings.TrimSpace(input.ChangeNote),
	}

	if err := s.definitions.UpdateStatus(ctx, tenantUUID, input.DefinitionUUID, definition.Version, "published", update); err != nil {
		return nil, err
	}

	definition, err = s.definitions.GetByUUID(ctx, tenantUUID, input.DefinitionUUID, &definition.Version)
	if err != nil {
		return nil, err
	}

	s.em.emit(ctx, newWorkflowEvent(
		definition.TenantUUID,
		uuid.Nil,
		"workflow.definition.published",
		fmt.Sprintf("workflow %s@v%d published", definition.Name, definition.Version),
		map[string]any{"definition_uuid": definition.UUID.String()},
	))

	return definition, nil
}

// StartInstanceInput 定义启动实例的入参。
type StartInstanceInput struct {
	TenantUUID        string
	DefinitionUUID    uuid.UUID
	DefinitionVersion int32
	Initiator         uuid.UUID
	Input             map[string]any
	Tags              map[string]string
	CorrelationID     string
}

// StartInstance 启动新的工作流实例。
func (s *Service) StartInstance(ctx context.Context, input StartInstanceInput) (*modelworkflow.WorkflowInstance, error) {
	if s == nil {
		return nil, errors.New("workflow service unavailable")
	}
	tenantUUID, err := normalizeTenantUUID(input.TenantUUID)
	if err != nil {
		return nil, err
	}
	if input.DefinitionUUID == uuid.Nil {
		return nil, errors.New("definition_id is required")
	}

	var versionPtr *int32
	if input.DefinitionVersion > 0 {
		versionPtr = &input.DefinitionVersion
	}
	definition, err := s.definitions.GetByUUID(ctx, tenantUUID, input.DefinitionUUID, versionPtr)
	if err != nil {
		return nil, err
	}
	if definition.Status != "published" {
		return nil, fmt.Errorf("definition %s is not published", definition.UUID)
	}

	steps, err := loadStepGraph(definition.StepGraph)
	if err != nil {
		return nil, err
	}
	validation, err := ValidateStepDefinitions(steps)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()

	instance := &modelworkflow.WorkflowInstance{
		TenantUUID:        tenantUUID,
		DefinitionUUID:    definition.UUID,
		DefinitionVersion: definition.Version,
		State:             "running",
		InputContext:      toJSONOrEmpty(input.Input),
		OutputContext:     datatypes.JSON([]byte(`{}`)),
		SlaSnapshot:       datatypes.JSON([]byte(`{}`)),
		CorrelationID:     strings.TrimSpace(input.CorrelationID),
		Tags:              toJSONOrEmpty(input.Tags),
		SlaDeadline:       nil,
		StartedAt:         &now,
		NextHeartbeatDue:  now,
		LastTransitionAt:  now,
	}
	if len(validation.StartStepIDs) > 0 {
		instance.CurrentStepID = validation.StartStepIDs[0]
	}

	instance, err = s.instances.CreateInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	for _, stepID := range validation.StartStepIDs {
		stepDef, ok := validation.StepByID(stepID)
		if !ok {
			return nil, fmt.Errorf("unknown start step %s", stepID)
		}
		exec, err := DefaultExecutorRouter().Executor(stepDef.Type)
		if err != nil {
			return nil, err
		}
		subjectType := exec.SubjectType()
		if subjectType == "" {
			subjectType = "system"
		}
		record := &modelworkflow.WorkflowStepRecord{
			InstanceUUID:   instance.UUID,
			StepID:         stepID,
			Type:           stepDef.Type,
			SubjectType:    subjectType,
			State:          "queued",
			ScheduledAt:    now,
			LastTransition: now,
		}
		if subjectType == "human" {
			record.AwaitingHuman = true
		}
		created, err := s.steps.AppendRecord(ctx, record)
		if err != nil {
			return nil, err
		}
		if s.tracker != nil && strings.EqualFold(stepDef.Type, "agent") {
			agentID, capability := extractAgentConfig(stepDef)
			if agentID != uuid.Nil {
				if _, err := s.tracker.Dispatch(ctx, AssignmentDispatchInput{
					TenantUUID:   input.TenantUUID,
					InstanceUUID: instance.UUID,
					StepRecordID: created.ID,
					StepID:       stepDef.ID,
					AgentUUID:    agentID,
					Capability:   capability,
				}); err != nil {
					return nil, err
				}
			}
		}
	}

	s.em.emit(ctx, newWorkflowEvent(
		instance.TenantUUID,
		instance.UUID,
		"workflow.instance.started",
		fmt.Sprintf("workflow instance %s started", instance.UUID.String()),
		map[string]any{
			"definition_uuid": instance.DefinitionUUID.String(),
			"version":         instance.DefinitionVersion,
		},
	))

	return instance, nil
}

// ControlInstanceInput 描述人工控制动作。
type ControlInstanceInput struct {
	TenantUUID   string
	InstanceUUID uuid.UUID
	Action       string
	Operator     uuid.UUID
	StepID       string
	AssignmentID uint64
	Reason       string
	Payload      map[string]any
}

// GetDefinition 按租户与 UUID 获取工作流定义，支持可选版本过滤。
func (s *Service) GetDefinition(ctx context.Context, tenantUUID string, definitionUUID uuid.UUID, version *int32) (*modelworkflow.WorkflowDefinition, error) {
	if s == nil {
		return nil, errors.New("workflow service unavailable")
	}
	return s.definitions.GetByUUID(ctx, tenantUUID, definitionUUID, version)
}

// ListDefinitions 分页查询工作流定义。
func (s *Service) ListDefinitions(ctx context.Context, tenantUUID string, status []string, keyword string, limit, offset int) ([]modelworkflow.WorkflowDefinition, int64, error) {
	if s == nil {
		return nil, 0, errors.New("workflow service unavailable")
	}
	return s.definitions.ListByTenant(ctx, tenantUUID, status, keyword, limit, offset)
}

// GetInstance 获取实例及可选的步骤列表。
func (s *Service) GetInstance(ctx context.Context, tenantUUID string, instanceUUID uuid.UUID, includeSteps bool) (*modelworkflow.WorkflowInstance, []modelworkflow.WorkflowStepRecord, error) {
	if s == nil {
		return nil, nil, errors.New("workflow service unavailable")
	}
	instance, err := s.instances.GetByUUID(ctx, tenantUUID, instanceUUID)
	if err != nil {
		return nil, nil, err
	}
	if !includeSteps {
		return instance, nil, nil
	}
	records, err := s.steps.ListByInstance(ctx, instance.UUID)
	return instance, records, err
}

// ListInstances 按条件分页查询实例。
func (s *Service) ListInstances(ctx context.Context, filter workflowrepo.InstanceListFilter) ([]modelworkflow.WorkflowInstance, int64, error) {
	if s == nil {
		return nil, 0, errors.New("workflow service unavailable")
	}
	return s.instances.ListInstances(ctx, filter)
}

type eventRepositoryAdapter struct {
	repo *workflowrepo.EventRepository
}

func (a *eventRepositoryAdapter) RecordEvent(ctx context.Context, evt *modelworkflow.WorkflowEvent) error {
	if a == nil || a.repo == nil || evt == nil {
		return nil
	}
	_, err := a.repo.Record(ctx, evt)
	return err
}

func toJSONOrEmpty(v any) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte(`{}`))
	}
	bytes, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON([]byte(`{}`))
	}
	if len(bytes) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return datatypes.JSON(bytes)
}

func loadStepGraph(jsonData datatypes.JSON) ([]StepDefinition, error) {
	if len(jsonData) == 0 {
		return nil, errors.New("step graph is empty")
	}
	var steps []StepDefinition
	if err := json.Unmarshal(jsonData, &steps); err != nil {
		return nil, fmt.Errorf("decode step graph failed: %w", err)
	}
	return steps, nil
}

func extractAgentConfig(step StepDefinition) (uuid.UUID, string) {
	var agentID uuid.UUID
	capability := ""
	if step.Config != nil {
		if v, ok := step.Config["agent_id"].(string); ok {
			if parsed, err := uuid.Parse(strings.TrimSpace(v)); err == nil {
				agentID = parsed
			}
		}
		if v, ok := step.Config["capability"].(string); ok {
			capability = strings.TrimSpace(v)
		}
	}
	return agentID, capability
}

func (s *Service) requireTenantUUID(tenantUUID string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(tenantUUID))
	if trimmed == "" {
		return "", errors.New("tenant uuid is required")
	}
	return trimmed, nil
}
