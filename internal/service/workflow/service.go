package workflow

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"

	"github.com/ArtisanCloud/PowerX/internal/service"
)

// ErrNotImplemented 表示后续阶段才会补全的业务逻辑。
var ErrNotImplemented = errors.New("workflow service: not implemented")

// DefinitionStore 定义层持久化接口。
type DefinitionStore interface {
	CreateDefinition(ctx context.Context, def *modelworkflow.WorkflowDefinition) (*modelworkflow.WorkflowDefinition, error)
	NextVersion(ctx context.Context, tenantID uint64, name string) (int32, error)
	GetByUUID(ctx context.Context, tenantID uint64, definitionUUID uuid.UUID, version *int32) (*modelworkflow.WorkflowDefinition, error)
	GetLatestPublished(ctx context.Context, tenantID uint64, definitionUUID uuid.UUID) (*modelworkflow.WorkflowDefinition, error)
	ListByTenant(ctx context.Context, tenantID uint64, status []string, keyword string, limit, offset int) ([]modelworkflow.WorkflowDefinition, int64, error)
	UpdateStatus(ctx context.Context, tenantID uint64, definitionUUID uuid.UUID, version int32, status string, updates map[string]interface{}) error
}

// InstanceStore 实例层持久化接口。
type InstanceStore interface {
	CreateInstance(ctx context.Context, instance *modelworkflow.WorkflowInstance) (*modelworkflow.WorkflowInstance, error)
	GetByUUID(ctx context.Context, tenantID uint64, instanceUUID uuid.UUID) (*modelworkflow.WorkflowInstance, error)
	ListInstances(ctx context.Context, filter workflowrepo.InstanceListFilter) ([]modelworkflow.WorkflowInstance, int64, error)
	UpdateState(ctx context.Context, tenantID uint64, instanceUUID uuid.UUID, nextState string, updates map[string]interface{}) error
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
}

// EventRecorder 记录工作流事件。
type EventRecorder interface {
	RecordEvent(ctx context.Context, evt *modelworkflow.WorkflowEvent) error
}

// Service 聚合工作流定义、实例与调度相关逻辑。
type Service struct {
	*service.BaseService

	definitions DefinitionStore
	instances   InstanceStore
	steps       StepRecordStore
	assignments AssignmentStore
	events      EventRecorder

	now func() time.Time
}

// ServiceOptions 用于注入自定义依赖。
type ServiceOptions struct {
	DefinitionStore DefinitionStore
	InstanceStore   InstanceStore
	StepStore       StepRecordStore
	AssignmentStore AssignmentStore
	EventRecorder   EventRecorder
	Clock           func() time.Time
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

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	return &Service{
		BaseService: &service.BaseService{DB: db},
		definitions: defStore,
		instances:   instStore,
		steps:       stepStore,
		assignments: assignStore,
		events:      opts.EventRecorder,
		now:         clock,
	}
}

// CreateDefinitionInput 定义创建工作流所需参数。
type CreateDefinitionInput struct {
	TenantID    uint64
	Name        string
	Description string
	CreatedBy   uuid.UUID
	StepGraph   map[string]any
	Metadata    map[string]any
}

// PublishDefinitionInput 定义发布工作流所需参数。
type PublishDefinitionInput struct {
	TenantID       uint64
	DefinitionUUID uuid.UUID
	Version        int32
	PublishedBy    uuid.UUID
	ChangeNote     string
}

// CreateDefinition 创建工作流定义（后续阶段补全具体逻辑）。
func (s *Service) CreateDefinition(ctx context.Context, input CreateDefinitionInput) (*modelworkflow.WorkflowDefinition, error) {
	return nil, ErrNotImplemented
}

// PublishDefinition 发布指定版本的工作流定义。
func (s *Service) PublishDefinition(ctx context.Context, input PublishDefinitionInput) (*modelworkflow.WorkflowDefinition, error) {
	return nil, ErrNotImplemented
}

// StartInstanceInput 定义启动实例的入参。
type StartInstanceInput struct {
	TenantID          uint64
	DefinitionUUID    uuid.UUID
	DefinitionVersion int32
	Initiator         uuid.UUID
	Input             map[string]any
	Tags              map[string]string
	CorrelationID     string
}

// StartInstance 启动新的工作流实例。
func (s *Service) StartInstance(ctx context.Context, input StartInstanceInput) (*modelworkflow.WorkflowInstance, error) {
	return nil, ErrNotImplemented
}

// ControlInstanceInput 描述人工控制动作。
type ControlInstanceInput struct {
	TenantID     uint64
	InstanceUUID uuid.UUID
	Action       string
	Operator     uuid.UUID
	StepID       string
	AssignmentID uint64
	Reason       string
	Payload      map[string]any
}

// ControlInstance 执行暂停、恢复、取消等操作。
func (s *Service) ControlInstance(ctx context.Context, input ControlInstanceInput) error {
	return ErrNotImplemented
}
