package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
)

var ErrHumanReviewStoreUnavailable = errors.New("workflow.human_review_store_unavailable")

type HumanReviewAdapter struct {
	reviews HumanReviewStore
}

func NewHumanReviewAdapter(reviews HumanReviewStore) *HumanReviewAdapter {
	return &HumanReviewAdapter{reviews: reviews}
}

func (a *HumanReviewAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "human.review",
		DisplayName:  "workflow.node.human.review",
		Category:     "human",
		InputSchema:  requiredObjectSchema("review_type", "approver_policy", "review_payload_path", "approved_route", "rejected_route"),
		OutputSchema: objectSchema(),
	}
}

func (a *HumanReviewAdapter) Validate(step StepDefinition) error {
	if err := requireConfigString(step, "review_type"); err != nil {
		return err
	}
	if err := requireConfigValue(step, "approver_policy"); err != nil {
		return err
	}
	if err := requireConfigString(step, "review_payload_path"); err != nil {
		return err
	}
	if _, err := singleRoute(step.Config["approved_route"]); err != nil {
		return fmt.Errorf("workflow.node_config_invalid: %s.approved_route", step.ID)
	}
	if _, err := singleRoute(step.Config["rejected_route"]); err != nil {
		return fmt.Errorf("workflow.node_config_invalid: %s.rejected_route", step.ID)
	}
	return nil
}

func (a *HumanReviewAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	if a == nil || a.reviews == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrHumanReviewStoreUnavailable.Error()}, ErrHumanReviewStoreUnavailable
	}
	if exec.Instance == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.instance_required"}, errors.New("workflow.instance_required")
	}
	task := &modelworkflow.HumanReviewTask{
		TenantUUID:           strings.ToLower(strings.TrimSpace(exec.TenantUUID)),
		WorkflowInstanceUUID: exec.Instance.UUID,
		StepID:               exec.Step.ID,
		ReviewType:           configString(exec.Step.Config, "review_type"),
		Payload:              toJSONOrEmpty(exec.Payload),
		ApproverPolicy:       toJSONOrEmpty(exec.Step.Config["approver_policy"]),
		Status:               "pending",
	}
	created, err := a.reviews.CreateTask(ctx, task)
	if err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.human_review_create_failed", ErrorMessage: err.Error()}, err
	}
	return NodeResult{
		Status:         NodeResultStatusWaiting,
		Output:         map[string]any{"review_task_uuid": created.UUID.String()},
		AwaitingHuman:  true,
		ReviewTaskUUID: created.UUID,
	}, nil
}

type HumanReviewActionInput struct {
	TenantUUID     string
	ReviewTaskUUID uuid.UUID
	Action         string
	ReviewerUUID   uuid.UUID
	Comment        string
	Payload        map[string]any
}

type HumanReviewListInput struct {
	TenantUUID           string
	Status               string
	WorkflowInstanceUUID uuid.UUID
	ReviewType           string
	Page                 int
	PageSize             int
}

func (s *Service) ListHumanReviewTasks(ctx context.Context, input HumanReviewListInput) ([]modelworkflow.HumanReviewTask, int64, error) {
	if s == nil || s.reviews == nil {
		return nil, 0, ErrHumanReviewStoreUnavailable
	}
	tenantUUID, err := normalizeTenantUUID(input.TenantUUID)
	if err != nil {
		return nil, 0, err
	}
	return s.reviews.ListTasks(ctx, workflowrepo.HumanReviewTaskListFilter{
		TenantUUID:           tenantUUID,
		Status:               input.Status,
		WorkflowInstanceUUID: input.WorkflowInstanceUUID,
		ReviewType:           input.ReviewType,
		Page:                 input.Page,
		PageSize:             input.PageSize,
	})
}

func (s *Service) GetHumanReviewTask(ctx context.Context, tenantUUID string, taskUUID uuid.UUID) (*modelworkflow.HumanReviewTask, error) {
	if s == nil || s.reviews == nil {
		return nil, ErrHumanReviewStoreUnavailable
	}
	normalizedTenantUUID, err := normalizeTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	return s.reviews.GetByUUID(ctx, normalizedTenantUUID, taskUUID)
}

func (s *Service) ActHumanReviewTask(ctx context.Context, input HumanReviewActionInput) (*modelworkflow.HumanReviewTask, error) {
	if s == nil || s.reviews == nil {
		return nil, ErrHumanReviewStoreUnavailable
	}
	tenantUUID, err := normalizeTenantUUID(input.TenantUUID)
	if err != nil {
		return nil, err
	}
	if input.ReviewTaskUUID == uuid.Nil {
		return nil, errors.New("workflow.review_task_uuid_required")
	}
	if input.ReviewerUUID == uuid.Nil {
		return nil, errors.New("workflow.reviewer_uuid_required")
	}
	action := normalizeHumanReviewAction(input.Action)
	if action == "" {
		return nil, errors.New("workflow.review_action_invalid")
	}

	task, err := s.reviews.GetByUUID(ctx, tenantUUID, input.ReviewTaskUUID)
	if err != nil {
		return nil, err
	}
	if task.Status != "pending" {
		return nil, fmt.Errorf("workflow.review_task_not_pending: %s", task.Status)
	}

	status := humanReviewStatusForAction(action)
	now := s.now().UTC()
	updates := map[string]interface{}{
		"reviewer_user_uuid": input.ReviewerUUID,
		"decision":           action,
		"decision_payload":   toJSONOrEmpty(input.Payload),
		"comment":            strings.TrimSpace(input.Comment),
		"completed_at":       now,
	}
	if err := s.reviews.UpdateDecision(ctx, tenantUUID, input.ReviewTaskUUID, status, updates); err != nil {
		return nil, err
	}
	if err := s.applyHumanReviewDecision(ctx, task, action, input.Payload); err != nil {
		return nil, err
	}
	return s.reviews.GetByUUID(ctx, tenantUUID, input.ReviewTaskUUID)
}

func (s *Service) applyHumanReviewDecision(ctx context.Context, task *modelworkflow.HumanReviewTask, action string, payload map[string]any) error {
	instance, err := s.instances.GetByUUID(ctx, task.TenantUUID, task.WorkflowInstanceUUID)
	if err != nil {
		return err
	}
	_, validation, err := s.loadDefinitionContext(ctx, instance)
	if err != nil {
		return err
	}
	step, ok := validation.StepByID(task.StepID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrWorkflowStepDefinitionNil, task.StepID)
	}
	record, err := s.steps.FindLatestByStep(ctx, instance.UUID, task.StepID)
	if err != nil {
		return err
	}
	if record.State != "waiting" {
		return fmt.Errorf("workflow.review_step_not_waiting: %s", record.State)
	}

	approved := action == "approve"
	if action == "cancel" {
		if err := s.steps.UpdateState(ctx, record.ID, "failed", map[string]interface{}{
			"awaiting_human": false,
			"error_code":     "workflow.human_review_canceled",
			"error_message":  "workflow.human_review_canceled",
		}); err != nil {
			return err
		}
		if err := s.instances.UpdateState(ctx, instance.TenantUUID, instance.UUID, "failed", map[string]interface{}{
			"current_step_id": record.StepID,
			"last_error":      "workflow.human_review_canceled",
			"completed_at":    s.now().UTC(),
		}); err != nil {
			return err
		}
		s.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.step.failed", record.StepID, map[string]any{
			"step_record_id": record.ID,
			"error_code":     "workflow.human_review_canceled",
		})
		return nil
	}

	result := StepResult{
		Status:      NodeResultStatusSucceeded,
		Decision:    action,
		Output:      cloneMap(payload),
		Approved:    approved,
		HasApproval: true,
	}
	nextStepIDs, err := DefaultExecutorRouter().NextSteps(step, result)
	if err != nil {
		return err
	}
	if err := s.steps.UpdateState(ctx, record.ID, "completed", map[string]interface{}{
		"payload_out":    toJSONOrEmpty(payload),
		"awaiting_human": false,
	}); err != nil {
		return err
	}
	s.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.step.completed", record.StepID, map[string]any{
		"step_record_id": record.ID,
		"decision":       action,
	})
	runner := &Runner{
		instances: s.instances,
		steps:     s.steps,
		router:    DefaultExecutorRouter(),
		now:       s.now,
		publish:   s.publishRuntimeEvent,
	}
	if err := runner.enqueueNextSteps(ctx, instance, validation, nextStepIDs); err != nil {
		return err
	}
	return runner.convergeInstanceState(ctx, instance)
}

func normalizeHumanReviewAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "approve", "approved":
		return "approve"
	case "reject", "rejected":
		return "reject"
	case "request_changes", "changes_requested":
		return "request_changes"
	case "cancel", "canceled":
		return "cancel"
	default:
		return ""
	}
}

func humanReviewStatusForAction(action string) string {
	switch action {
	case "approve":
		return "approved"
	case "reject":
		return "rejected"
	case "request_changes":
		return "changes_requested"
	case "cancel":
		return "canceled"
	default:
		return ""
	}
}
