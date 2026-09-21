package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type VerificationEvidenceService struct {
	repo *repository.VerificationEvidenceRepository
}

type verificationEvidenceServiceContextKey struct{}

func NewVerificationEvidenceService(db *gorm.DB) *VerificationEvidenceService {
	return &VerificationEvidenceService{repo: repository.NewVerificationEvidenceRepository(db)}
}

func ContextWithVerificationEvidenceService(ctx context.Context, service *VerificationEvidenceService) context.Context {
	if ctx == nil || service == nil {
		return ctx
	}
	return context.WithValue(ctx, verificationEvidenceServiceContextKey{}, service)
}

func VerificationEvidenceServiceFromContext(ctx context.Context) (*VerificationEvidenceService, bool) {
	if ctx == nil {
		return nil, false
	}
	service, ok := ctx.Value(verificationEvidenceServiceContextKey{}).(*VerificationEvidenceService)
	return service, ok && service != nil
}

func (s *VerificationEvidenceService) Persist(ctx context.Context, env, tenantUUID string, runUUID uuid.UUID, snapshotUUID uuid.UUID, verdict VerificationVerdict, artifactRef string) error {
	if s == nil || s.repo == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || runUUID == uuid.Nil || snapshotUUID == uuid.Nil || strings.TrimSpace(verdict.Class) == "" || strings.TrimSpace(verdict.ReasonCode) == "" {
		return fmt.Errorf("verification evidence input is incomplete")
	}
	taskRefs, err := json.Marshal(verdict.TaskRefs)
	if err != nil {
		return fmt.Errorf("marshal verification task refs: %w", err)
	}
	inputFields, err := json.Marshal(verdict.RequiredInputFields)
	if err != nil {
		return fmt.Errorf("marshal verification input fields: %w", err)
	}
	permissionCodes, err := json.Marshal(verdict.RequiredPermissionCodes)
	if err != nil {
		return fmt.Errorf("marshal verification permission codes: %w", err)
	}
	return s.repo.Create(ctx, &dbmodel.AgentVerificationEvidence{Env: strings.TrimSpace(env), TenantUUID: strings.TrimSpace(tenantUUID), RunUUID: runUUID, SnapshotUUID: snapshotUUID, TaskRefs: datatypes.JSON(taskRefs), VerdictClass: verdict.Class, ReasonCode: verdict.ReasonCode, UserAction: verdict.UserAction, RequiredInputFields: datatypes.JSON(inputFields), RequiredPermissionCodes: datatypes.JSON(permissionCodes), ArtifactRef: strings.TrimSpace(artifactRef)})
}

// PersistVerifiedCapabilityTask stores the immutable side-effect evidence for
// one completed tooling task. A run-level verdict is intentionally not used as
// a substitute: a plan can contain several independently auditable effects.
func (s *VerificationEvidenceService) PersistVerifiedCapabilityTask(ctx context.Context, env, tenantUUID string, runUUID uuid.UUID, snapshot *ResourceSnapshot, task flowschema.PlanTask, out *agentschema.ExecutionResult) error {
	if snapshot == nil || !strings.EqualFold(strings.TrimSpace(task.NodeKind), "tooling") || out == nil || !out.Success {
		return nil
	}
	capabilityUUID, err := taskCapabilityUUID(task)
	if err != nil {
		return err
	}
	capability, err := snapshot.RequireInvocable(capabilityUUID)
	if err != nil {
		return err
	}
	contract := capability.RuntimeContract
	if !contract.VerificationRequired {
		return nil
	}
	if out.Metadata == nil {
		return fmt.Errorf("capability verification evidence is required")
	}
	record, ok := out.Metadata["capability_verification"].(map[string]any)
	if !ok {
		return fmt.Errorf("capability verification evidence must be a structured object")
	}
	artifactRef := strings.TrimSpace(anyToString(record["artifact_ref"]))
	if strings.TrimSpace(anyToString(record["schema"])) != contract.SideEffectEvidenceSchema || !strings.HasPrefix(artifactRef, "capability_verification/"+capabilityUUID.String()+"/") {
		return fmt.Errorf("capability verification evidence does not match frozen contract")
	}
	return s.Persist(ctx, env, tenantUUID, runUUID, snapshot.SnapshotUUID, VerificationVerdict{
		Class:      VerificationPass,
		ReasonCode: "capability.verified",
		TaskRefs:   []string{task.TaskID},
	}, artifactRef)
}

func verificationArtifactRef(report *agent.PlanExecutionReport, out *agentschema.ExecutionResult) string {
	if out != nil && out.Metadata != nil {
		if ref := strings.TrimSpace(anyToString(out.Metadata["verification_artifact_ref"])); ref != "" {
			return ref
		}
	}
	// A tooling task's verification artifact is authoritative for a side effect;
	// the final response result normally has no copy of that metadata. Persist a
	// single unambiguous task artifact rather than silently dropping it or
	// selecting one arbitrarily from a multi-capability plan.
	refs := map[string]struct{}{}
	if report != nil {
		for _, task := range report.Tasks {
			if task.Result == nil || task.Result.Metadata == nil {
				continue
			}
			record, ok := task.Result.Metadata["capability_verification"].(map[string]any)
			if !ok {
				continue
			}
			if ref := strings.TrimSpace(anyToString(record["artifact_ref"])); ref != "" {
				refs[ref] = struct{}{}
			}
		}
	}
	if len(refs) != 1 {
		return ""
	}
	for ref := range refs {
		return ref
	}
	return ""
}
