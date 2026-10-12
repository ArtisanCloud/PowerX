package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	runtimescheduler "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

type SchedulerCompletionVerifier struct{ service *runtimescheduler.Service }

func NewSchedulerCompletionVerifier(service *runtimescheduler.Service) SchedulerCompletionVerifier {
	return SchedulerCompletionVerifier{service: service}
}
func (v SchedulerCompletionVerifier) VerifyCompletion(ctx context.Context, resource ResourceDescriptor, _ flowschema.PlanTask, out *agentschema.ExecutionResult) (string, error) {
	if v.service == nil || out == nil || out.Data == nil || resource.CapabilityID != runtimescheduler.CapabilityID {
		return "", fmt.Errorf("scheduler completion verification is invalid")
	}
	result, ok := out.Data["result"]
	if !ok {
		return "", fmt.Errorf("scheduler invocation result is required")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	var payload struct {
		Job struct {
			UUID   string `json:"uuid"`
			Status string `json:"status"`
		} `json:"job"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil || strings.TrimSpace(payload.Job.UUID) == "" || strings.TrimSpace(payload.Job.Status) == "" {
		return "", fmt.Errorf("scheduler result job uuid and status are required")
	}
	job, err := v.service.GetJob(ctx, payload.Job.UUID)
	if err != nil || job == nil || job.UUID.String() != payload.Job.UUID || strings.TrimSpace(job.Status) != strings.TrimSpace(payload.Job.Status) {
		return "", fmt.Errorf("scheduler job readback does not match invocation result")
	}
	return "scheduler_job/" + job.UUID.String() + "/" + job.Status, nil
}
