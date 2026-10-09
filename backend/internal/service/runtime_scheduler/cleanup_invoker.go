package runtimescheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

type CleanupInvoker struct{ service *Service }

func NewCleanupInvoker(service *Service) *CleanupInvoker { return &CleanupInvoker{service: service} }
func cleanupDecode(body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var tail any
	if err := decoder.Decode(&tail); err != io.EOF {
		return appErr(400, "SCHEDULER_INVALID_ARGUMENT", "仅接受一个typed对象", err)
	}
	return nil
}
func (i *CleanupInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	if in.CapabilityID != CleanupReadCapabilityID && in.CapabilityID != CleanupDeleteCapabilityID {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	invalid := func() (map[string]any, error) {
		return nil, appErr(400, "SCHEDULER_INVALID_ARGUMENT", "只接受固定typed调度操作", nil)
	}
	if in.Method != "INVOKE" || in.Endpoint != "core://scheduler/jobs" || len(in.Query) != 0 {
		return invalid()
	}
	for key := range in.Payload {
		if key != "body" && key != "method" && key != "endpoint" {
			return invalid()
		}
	}
	if !cap.ServiceCredential(ctx) || reqctx.GetTenantUUID(ctx) != in.TenantUUID {
		return nil, appErr(403, "SCHEDULER_SERVICE_ACTOR_REQUIRED", "需要可信服务身份", nil)
	}
	if i == nil || i.service == nil {
		return nil, appErr(503, "SCHEDULER_UNAVAILABLE", "调度服务不可用", nil)
	}
	raw, _ := json.Marshal(in.Body)
	var operation struct {
		Operation string `json:"operation"`
	}
	if json.Unmarshal(raw, &operation) != nil {
		return invalid()
	}
	if in.CapabilityID == CleanupDeleteCapabilityID {
		var request struct {
			Operation string `json:"operation"`
			DeleteJobInput
		}
		if cleanupDecode(in.Body, &request) != nil || request.Operation != "delete_job" {
			return invalid()
		}
		result, err := i.service.DeleteJob(ctx, request.DeleteJobInput)
		return map[string]any{"result": result}, err
	}
	switch operation.Operation {
	case "list_jobs":
		var request struct {
			Operation string `json:"operation"`
			OwnerType string `json:"owner_type,omitempty"`
			OwnerID   string `json:"owner_id"`
			Status    string `json:"status,omitempty"`
			Page      int    `json:"page,omitempty"`
			PageSize  int    `json:"page_size,omitempty"`
		}
		if cleanupDecode(in.Body, &request) != nil {
			return invalid()
		}
		items, total, err := i.service.ListCleanupJobs(ctx, ListJobsInput{OwnerType: request.OwnerType, OwnerID: request.OwnerID, Status: request.Status, Page: request.Page, PageSize: request.PageSize})
		page, size := normalizePage(request.Page, request.PageSize)
		return map[string]any{"items": items, "pagination": map[string]any{"total": total, "page": page, "page_size": size}}, err
	case "get_job":
		var request struct {
			Operation      string `json:"operation"`
			JobUUID        string `json:"job_uuid"`
			IncludeDeleted bool   `json:"include_deleted,omitempty"`
		}
		if cleanupDecode(in.Body, &request) != nil {
			return invalid()
		}
		item, err := i.service.GetCleanupJob(ctx, request.JobUUID, request.IncludeDeleted)
		return map[string]any{"item": item}, err
	case "list_runs":
		var request struct {
			Operation string `json:"operation"`
			JobUUID   string `json:"job_uuid"`
			Page      int    `json:"page,omitempty"`
			PageSize  int    `json:"page_size,omitempty"`
		}
		if cleanupDecode(in.Body, &request) != nil {
			return invalid()
		}
		items, total, err := i.service.ListCleanupRuns(ctx, ListRunsInput{JobID: request.JobUUID, Page: request.Page, PageSize: request.PageSize})
		page, size := normalizePage(request.Page, request.PageSize)
		return map[string]any{"items": items, "pagination": map[string]any{"total": total, "page": page, "page_size": size}}, err
	default:
		return invalid()
	}
}
