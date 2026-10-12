package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
)

const maxExecutionPlanArtifactBytes = 16 << 20

var artifactEnvPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
var artifactChecksumPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type executionPlanArtifact struct {
	SchemaVersion int                      `json:"schema_version"`
	TenantUUID    string                   `json:"tenant_uuid"`
	Env           string                   `json:"env"`
	RunID         string                   `json:"run_id"`
	Checksum      string                   `json:"checksum"`
	Plan          flowschema.ExecutionPlan `json:"plan"`
}

func validPlanArtifactRef(ref agent_run.TaskRef) bool {
	tenant, tenantErr := uuid.Parse(ref.TenantUUID)
	run, runErr := uuid.Parse(ref.RunID)
	return tenantErr == nil && runErr == nil && tenant != uuid.Nil && run != uuid.Nil &&
		tenant.String() == ref.TenantUUID && run.String() == ref.RunID && artifactEnvPattern.MatchString(ref.Env)
}

func executionPlanKey(ref agent_run.TaskRef, checksum string) string {
	return fmt.Sprintf("agent-runs/%s/%s/%s/plan-r1-%s.json", ref.TenantUUID, ref.Env, ref.RunID, checksum)
}

// SaveExecutionPlanArtifact verifies that object storage can read back the
// complete executable plan before RunStore publishes the scheduling DAG.
func SaveExecutionPlanArtifact(ctx context.Context, objects agent_run.ReportObjectStore,
	ref agent_run.TaskRef, plan *flowschema.ExecutionPlan) (string, string, error) {
	if objects == nil || plan == nil || len(plan.Tasks) == 0 || !validPlanArtifactRef(ref) {
		return "", "", agent_run.ErrInvalid
	}
	planBody, err := json.Marshal(plan)
	if err != nil || len(planBody) > maxExecutionPlanArtifactBytes {
		return "", "", agent_run.ErrInvalid
	}
	sum := sha256.Sum256(planBody)
	checksum := hex.EncodeToString(sum[:])
	artifact := executionPlanArtifact{SchemaVersion: 1, TenantUUID: ref.TenantUUID, Env: ref.Env,
		RunID: ref.RunID, Checksum: checksum, Plan: *plan}
	body, err := json.Marshal(artifact)
	if err != nil || len(body) > maxExecutionPlanArtifactBytes {
		return "", "", agent_run.ErrInvalid
	}
	key := executionPlanKey(ref, checksum)
	if err := objects.Put(ctx, key, body); err != nil {
		return "", "", err
	}
	read, err := objects.Get(ctx, key)
	if err != nil {
		return "", "", fmt.Errorf("plan artifact readback failed: %w", err)
	}
	if len(read) > maxExecutionPlanArtifactBytes {
		return "", "", agent_run.ErrInvalid
	}
	if sha256.Sum256(read) != sha256.Sum256(body) {
		return "", "", agent_run.ErrConflict
	}
	return key, "sha256:" + checksum, nil
}

// LoadExecutionPlanArtifact checks tenant, Run and checksum before a task
// executor can use the stored flow parameters.
func LoadExecutionPlanArtifact(ctx context.Context, objects agent_run.ReportObjectStore,
	ref agent_run.TaskRef, key, evidence string) (*flowschema.ExecutionPlan, error) {
	checksum := strings.TrimPrefix(evidence, "sha256:")
	if objects == nil || !validPlanArtifactRef(ref) || !strings.HasPrefix(evidence, "sha256:") ||
		!artifactChecksumPattern.MatchString(checksum) || key != executionPlanKey(ref, checksum) {
		return nil, agent_run.ErrInvalid
	}
	body, err := objects.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > maxExecutionPlanArtifactBytes {
		return nil, agent_run.ErrInvalid
	}
	var artifact executionPlanArtifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		return nil, err
	}
	planBody, err := json.Marshal(artifact.Plan)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(planBody)
	if artifact.SchemaVersion != 1 || artifact.TenantUUID != ref.TenantUUID || artifact.Env != ref.Env ||
		artifact.RunID != ref.RunID || artifact.Checksum != hex.EncodeToString(sum[:]) || evidence != "sha256:"+artifact.Checksum {
		return nil, agent_run.ErrInvalid
	}
	return &artifact.Plan, nil
}
