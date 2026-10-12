package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
)

type taskResultArtifact struct {
	SchemaVersion int                     `json:"schema_version"`
	TenantUUID    string                  `json:"tenant_uuid"`
	Env           string                  `json:"env"`
	RunID         string                  `json:"run_id"`
	Revision      uint64                  `json:"revision"`
	TaskID        string                  `json:"task_id"`
	Attempt       uint64                  `json:"attempt"`
	Checksum      string                  `json:"checksum"`
	Result        aschema.ExecutionResult `json:"result"`
}

var resultTaskIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$`)

func taskResultObjectKey(ref agent_run.TaskRef, checksum string) string {
	return fmt.Sprintf("agent-runs/%s/%s/%s/task-r%d-%s-a%d-%s.json",
		ref.TenantUUID, ref.Env, ref.RunID, ref.Revision, ref.TaskID, ref.Attempt, checksum)
}

func validResultRef(ref agent_run.TaskRef) bool {
	return validPlanArtifactRef(ref) && ref.Revision > 0 && ref.Attempt > 0 && resultTaskIDPattern.MatchString(ref.TaskID)
}

// SaveTaskResultArtifact verifies the complete result is readable before the
// Worker records its locator in RunStore. The key is immutable per content.
func SaveTaskResultArtifact(ctx context.Context, objects agent_run.ReportObjectStore,
	ref agent_run.TaskRef, result *aschema.ExecutionResult) (string, string, error) {
	if objects == nil || !validResultRef(ref) || result == nil || !result.Success {
		return "", "", agent_run.ErrInvalid
	}
	resultBody, err := json.Marshal(result)
	if err != nil || len(resultBody) > maxExecutionPlanArtifactBytes {
		return "", "", agent_run.ErrInvalid
	}
	sum := sha256.Sum256(resultBody)
	checksum := hex.EncodeToString(sum[:])
	artifact := taskResultArtifact{SchemaVersion: 1, TenantUUID: ref.TenantUUID, Env: ref.Env,
		RunID: ref.RunID, Revision: ref.Revision, TaskID: ref.TaskID, Attempt: ref.Attempt,
		Checksum: checksum, Result: *result}
	body, err := json.Marshal(artifact)
	if err != nil || len(body) > maxExecutionPlanArtifactBytes {
		return "", "", agent_run.ErrInvalid
	}
	key := taskResultObjectKey(ref, checksum)
	if err := objects.Put(ctx, key, body); err != nil {
		return "", "", err
	}
	read, err := objects.Get(ctx, key)
	if err != nil {
		return "", "", fmt.Errorf("task result readback failed: %w", err)
	}
	if sha256.Sum256(read) != sha256.Sum256(body) {
		return "", "", agent_run.ErrConflict
	}
	return key, "sha256:" + checksum, nil
}

// LoadTaskResultArtifact verifies both storage identity and result digest.
func LoadTaskResultArtifact(ctx context.Context, objects agent_run.ReportObjectStore,
	ref agent_run.TaskRef, key, evidence string) (*aschema.ExecutionResult, error) {
	checksum := strings.TrimPrefix(evidence, "sha256:")
	if objects == nil || !validResultRef(ref) || !strings.HasPrefix(evidence, "sha256:") ||
		!artifactChecksumPattern.MatchString(checksum) || key != taskResultObjectKey(ref, checksum) {
		return nil, agent_run.ErrInvalid
	}
	body, err := objects.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > maxExecutionPlanArtifactBytes {
		return nil, agent_run.ErrInvalid
	}
	var artifact taskResultArtifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		return nil, err
	}
	resultBody, err := json.Marshal(artifact.Result)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(resultBody)
	if artifact.SchemaVersion != 1 || artifact.TenantUUID != ref.TenantUUID || artifact.Env != ref.Env ||
		artifact.RunID != ref.RunID || artifact.Revision != ref.Revision || artifact.TaskID != ref.TaskID ||
		artifact.Attempt != ref.Attempt || artifact.Checksum != hex.EncodeToString(sum[:]) ||
		evidence != "sha256:"+artifact.Checksum || !artifact.Result.Success {
		return nil, agent_run.ErrInvalid
	}
	return &artifact.Result, nil
}
