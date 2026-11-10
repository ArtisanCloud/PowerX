package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	imagefactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/image"
	ttsfactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/tts"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func (s *MediaRuntime) Run(ctx context.Context) error {
	if s == nil || s.queue == nil {
		return MediaError(503, "AI_TASKBUS_UNAVAILABLE")
	}
	for ctx.Err() == nil {
		if err := s.queue.Recover(ctx, mediaQueueTenant, mediaSubscriber); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	worker := "ai-media-" + uuid.NewString()
	for ctx.Err() == nil {
		deliveries, err := s.queue.Dequeue(ctx, mediaQueueTenant, mediaSubscriber, "ai-media-workers", worker, 1, 3*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.WarnF(ctx, "AI media TaskBus dequeue failed")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		}
		for _, delivery := range deliveries {
			if err = s.handleMediaDelivery(ctx, delivery); err != nil && !errors.Is(err, event_bus.ErrLeaseStale) {
				logger.WarnF(ctx, "AI media delivery deferred task=%s", delivery.Message.ID)
			}
		}
	}
	return ctx.Err()
}
func (s *MediaRuntime) handleMediaDelivery(ctx context.Context, d event_bus.StreamTaskDelivery) error {
	var message mediaTaskMessage
	if json.Unmarshal(d.Message.Payload, &message) != nil || !validMediaTaskID(message.Task) || message.Tenant == "" {
		return s.queue.Reject(ctx, d, "AI_TASK_PAYLOAD_INVALID", true)
	}
	runCtx, cancel := context.WithTimeout(reqctx.WithTenantUUID(ctx, message.Tenant), 6*time.Minute)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if s.queue.Renew(runCtx, d) != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	err := s.processMediaDelivery(runCtx, d, message)
	if errors.Is(err, taskqueue.ErrTaskTerminal) {
		return s.queue.Ack(ctx, d)
	}
	return err
}
func (s *MediaRuntime) applyMedia(ctx context.Context, d event_bus.StreamTaskDelivery, message mediaTaskMessage, apply func(*m.MediaTask) error) error {
	err := s.queue.Apply(ctx, d, func(tx *gorm.DB) error { return s.repo.Update(ctx, tx, message.Tenant, message.Task, apply) })
	if err == nil {
		s.notify(ctx, message.Tenant, message.Task)
	}
	return err
}
func (s *MediaRuntime) finishMedia(ctx context.Context, d event_bus.StreamTaskDelivery, message mediaTaskMessage, status, code string, assets []MediaAssetResult) error {
	err := s.queue.Complete(ctx, d, func(tx *gorm.DB) error {
		return s.repo.Update(ctx, tx, message.Tenant, message.Task, func(row *m.MediaTask) error {
			if terminalMediaTask(row.Status) {
				return nil
			}
			row.Status = status
			if status == "succeeded" && row.CancelStatus == "requested" {
				row.CancelStatus = "unsupported"
			}
			row.Phase = "completed"
			row.ErrorCode = code
			now := time.Now().UTC()
			row.CompletedAt = &now
			if assets != nil {
				raw, e := json.Marshal(assets)
				if e != nil {
					return e
				}
				row.Result = datatypes.JSON(raw)
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	s.notify(ctx, message.Tenant, message.Task)
	return s.queue.Ack(ctx, d)
}
func (s *MediaRuntime) processMediaDelivery(ctx context.Context, d event_bus.StreamTaskDelivery, message mediaTaskMessage) error {
	row, err := s.repo.Get(ctx, message.Tenant, message.Task, "")
	if err != nil {
		return err
	}
	if terminalMediaTask(row.Status) {
		return s.finishMedia(ctx, d, message, row.Status, row.ErrorCode, nil)
	}
	if time.Since(row.CreatedAt) > 24*time.Hour {
		return s.finishMedia(ctx, d, message, "failed", "AI_TASK_DEADLINE_EXCEEDED", nil)
	}
	request, model, mc, err := s.resolveTask(ctx, row)
	if err != nil {
		return s.finishMedia(ctx, d, message, "failed", mediaErrorCode(err), nil)
	}
	if row.Phase == "submitting" {
		// 网络请求之前已持久化 submitting；没有句柄时绝不重新生成。
		return s.finishMedia(ctx, d, message, "failed", "AI_SUBMISSION_OUTCOME_UNKNOWN", nil)
	}
	if row.Phase == "queued" {
		claimed := false
		err = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error {
			if current.Status == "cancelled" {
				return nil
			}
			if current.Phase != "queued" {
				return event_bus.ErrLeaseStale
			}
			current.Status = "running"
			current.Phase = "submitting"
			claimed = true
			return nil
		})
		if err != nil {
			return err
		}
		if !claimed {
			return s.finishMedia(ctx, d, message, "cancelled", "", nil)
		}
		if request.Operation.Async {
			client, e := s.video(request.Operation.Driver)
			if e != nil {
				return s.finishMedia(ctx, d, message, "failed", "AI_ASYNC_UNSUPPORTED", nil)
			}
			response, e := client.Submit(ctx, videoTaskRequest(request, mc))
			if e != nil {
				return s.failMediaSubmission(ctx, d, message, e)
			}
			if response == nil || response.TaskID == "" {
				return s.finishMedia(ctx, d, message, "failed", "AI_SUBMISSION_OUTCOME_UNKNOWN", nil)
			}
			err = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error {
				current.ProviderTaskID = response.TaskID
				current.ProviderTraceID = response.TraceID
				current.Phase = "waiting_provider"
				return nil
			})
			if err != nil {
				return err
			}
			return s.queue.Defer(ctx, d, time.Now().Add(s.providerDelay), "provider_pending")
		}
		output, e := s.runSynchronousMedia(ctx, request, model, mc)
		if e != nil {
			return s.failMediaSubmission(ctx, d, message, e)
		}
		raw, _ := json.Marshal(output)
		if err = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error {
			current.ProviderOutput = datatypes.JSON(raw)
			current.ProviderTraceID = output.TraceID
			current.ProviderStatus = output.Status
			current.Phase = "materializing"
			if current.CancelStatus == "requested" {
				current.CancelStatus = "unsupported"
			}
			return nil
		}); err != nil {
			return err
		}
		row.ProviderOutput = datatypes.JSON(raw)
		row.Phase = "materializing"
	}
	if row.Phase == "waiting_provider" {
		client, e := s.video(request.Operation.Driver)
		if e != nil {
			return s.finishMedia(ctx, d, message, "failed", "AI_ASYNC_UNSUPPORTED", nil)
		}
		var state *contract.VideoTaskStatus
		if row.CancelStatus == "requested" {
			state, e = client.Cancel(ctx, row.ProviderTaskID, map[string]any{"config": mc})
			if e == nil && state != nil && state.CancelStatus != "" {
				if e = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error { current.CancelStatus = state.CancelStatus; return nil }); e != nil {
					return e
				}
			}
		}
		if e != nil || state == nil {
			state, e = client.Query(ctx, row.ProviderTaskID, map[string]any{"config": mc})
		}
		if e != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var upstream *dto.AppError
			if errors.As(e, &upstream) && upstream.HTTPCode >= 400 && upstream.HTTPCode < 500 && upstream.HTTPCode != 429 {
				return s.failMediaSubmission(ctx, d, message, e)
			}
			if row.ErrorCode != "AI_PROVIDER_QUERY_FAILED" {
				if updateErr := s.applyMedia(ctx, d, message, func(current *m.MediaTask) error {
					current.ErrorCode = "AI_PROVIDER_QUERY_FAILED"
					if upstream != nil {
						current.ProviderStatus = intFromAny(upstream.Details["upstream_status"])
						current.ProviderErrorCode = stringFromAny(upstream.Details["upstream_code"])
						current.ProviderTraceID = stringFromAny(upstream.Details["provider_trace_id"])
					}
					return nil
				}); updateErr != nil {
					return updateErr
				}
			}
			return s.queue.Defer(ctx, d, time.Now().Add(10*time.Second), "provider_query_failed")
		}
		if state == nil {
			return s.queue.Defer(ctx, d, time.Now().Add(10*time.Second), "provider_query_empty")
		}
		if row.ErrorCode == "AI_PROVIDER_QUERY_FAILED" {
			if err = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error {
				current.ErrorCode = ""
				current.ProviderStatus = 0
				current.ProviderErrorCode = ""
				return nil
			}); err != nil {
				return err
			}
		}
		if state.Status == "cancelled" {
			if err = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error { current.CancelStatus = "confirmed"; return nil }); err != nil {
				return err
			}
			return s.finishMedia(ctx, d, message, "cancelled", "", nil)
		}
		if state.TraceID != "" && (state.Status == "succeeded" || state.Status == "failed") {
			if e = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error { current.ProviderTraceID = state.TraceID; return nil }); e != nil {
				return e
			}
		}
		if state.Status == "failed" {
			if err = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error { current.ProviderErrorCode = state.ErrorCode; return nil }); err != nil {
				return err
			}
			return s.finishMedia(ctx, d, message, "failed", "AI_PROVIDER_TASK_FAILED", nil)
		}
		if state.Status != "succeeded" {
			return s.queue.Defer(ctx, d, time.Now().Add(s.providerDelay), "provider_pending")
		}
		output := mediaProviderOutput{Kind: "video", URLs: state.VideoURLs}
		raw, _ := json.Marshal(output)
		if err = s.applyMedia(ctx, d, message, func(current *m.MediaTask) error {
			current.ProviderOutput = datatypes.JSON(raw)
			current.Phase = "materializing"
			return nil
		}); err != nil {
			return err
		}
		row.ProviderOutput = datatypes.JSON(raw)
		row.Phase = "materializing"
	}
	if row.Phase != "materializing" {
		return s.finishMedia(ctx, d, message, "failed", "AI_TASK_STATE_INVALID", nil)
	}
	var output mediaProviderOutput
	if json.Unmarshal(row.ProviderOutput, &output) != nil {
		return s.finishMedia(ctx, d, message, "failed", "AI_OUTPUT_INVALID", nil)
	}
	assets, err := s.materialize(ctx, message.Tenant, message.Task, output)
	if err != nil {
		switch mediaErrorCode(err) {
		case "AI_OUTPUT_INVALID", "AI_OUTPUT_TYPE_MISMATCH", "AI_MEDIA_URL_INVALID", "AI_MEDIA_SIZE_INVALID", "AI_MEDIA_REDIRECT_LIMIT":
			return s.finishMedia(ctx, d, message, "failed", mediaErrorCode(err), nil)
		}
		// 转存可以安全重试；稳定资产 UUID 保证不会重新生成或重复入库。
		if row.ErrorCode != mediaErrorCode(err) {
			if e := s.applyMedia(ctx, d, message, func(current *m.MediaTask) error { current.ErrorCode = mediaErrorCode(err); return nil }); e != nil {
				return e
			}
		}
		return s.queue.Defer(ctx, d, time.Now().Add(15*time.Second), "media_transfer_pending")
	}
	return s.finishMedia(ctx, d, message, "succeeded", "", assets)
}
func (s *MediaRuntime) failMediaSubmission(ctx context.Context, d event_bus.StreamTaskDelivery, message mediaTaskMessage, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	code := "AI_SUBMISSION_OUTCOME_UNKNOWN"
	var business *dto.AppError
	if errors.As(err, &business) {
		code = business.Code
		if e := s.applyMedia(ctx, d, message, func(row *m.MediaTask) error {
			row.ProviderStatus = intFromAny(business.Details["upstream_status"])
			row.ProviderErrorCode = stringFromAny(business.Details["upstream_code"])
			row.ProviderTraceID = stringFromAny(business.Details["provider_trace_id"])
			return nil
		}); e != nil {
			return e
		}
	}
	return s.finishMedia(ctx, d, message, "failed", code, nil)
}
func videoTaskRequest(request *mediaTaskRequest, mc *aiconfig.ModelConfig) contract.VideoRequest {
	p := request.EffectiveParams
	runtime := map[string]any{"config": mc}
	for key, value := range p {
		runtime[key] = value
	}
	return contract.VideoRequest{Prompt: BuildPrompt(request.Inputs), Resolution: stringFromAny(p["resolution"]), MaxDurationS: intFromAny(p["max_duration_sec"]), RefImages: buildImageRefParts(request.Inputs), RefVideos: buildVideoRefParts(request.Inputs), Runtime: runtime}
}
func (s *MediaRuntime) runSynchronousMedia(ctx context.Context, request *mediaTaskRequest, model *catalog.MediaModel, mc *aiconfig.ModelConfig) (mediaProviderOutput, error) {
	p := request.EffectiveParams
	runtime := map[string]any{"config": mc}
	for k, v := range p {
		runtime[k] = v
	}
	switch request.Capability {
	case catalog.ImageGeneration:
		client, err := imagefactory.NewClient(request.Operation.Driver)
		if err != nil {
			return mediaProviderOutput{}, err
		}
		out, err := client.Generate(ctx, imageTaskRequest(request, mc))
		if err != nil {
			return mediaProviderOutput{}, err
		}
		if out == nil {
			return mediaProviderOutput{}, MediaError(502, "AI_OUTPUT_INVALID")
		}
		return mediaProviderOutput{Kind: "image", URLs: out.ImageURLs, Data: out.Images, TraceID: out.TraceID, Status: 200}, nil
	case catalog.SpeechSynthesis:
		client, err := ttsfactory.NewClient(request.Operation.Driver)
		if err != nil {
			return mediaProviderOutput{}, err
		}
		out, err := client.Synthesize(ctx, contract.TTSRequest{Text: BuildPrompt(request.Inputs), Voice: stringFromAny(p["voice"]), Speed: floatFromAny(p["speed"]), Format: stringFromAny(p["format"]), Runtime: runtime})
		if err != nil {
			return mediaProviderOutput{}, err
		}
		if out == nil {
			return mediaProviderOutput{}, MediaError(502, "AI_OUTPUT_INVALID")
		}
		result := mediaProviderOutput{Kind: "audio"}
		if len(out.Audio) > 0 {
			result.Data = [][]byte{out.Audio}
		} else if out.AudioURL != "" {
			result.URLs = []string{out.AudioURL}
		}
		return result, nil
	}
	return mediaProviderOutput{}, MediaError(422, "AI_CAPABILITY_INVALID")
}

func imageTaskRequest(request *mediaTaskRequest, mc *aiconfig.ModelConfig) contract.ImageRequest {
	p := request.EffectiveParams
	runtime := map[string]any{"config": mc}
	for key, value := range p {
		runtime[key] = value
	}
	return contract.ImageRequest{Prompt: BuildPrompt(request.Inputs), Size: stringFromAny(p["size"]), Quality: stringFromAny(p["quality"]), Format: stringFromAny(p["format"]), RefImages: buildImageRefParts(request.Inputs), Runtime: runtime}
}
