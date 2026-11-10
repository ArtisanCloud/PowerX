package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/redisnamespace"
	"regexp"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	videofactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/video"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const MediaTaskReadCapability = "com.corex.ai.media.tasks.read"
const MediaTaskCancelCapability = "com.corex.ai.media.tasks.cancel"
const mediaQueueTenant = "core-ai-media"
const mediaSubscriber = "media-generation-v2"
const MediaTaskChangedTopic = "ai.media.task.changed"

type MediaAssetResult struct {
	MediaAssetUUID    string `json:"media_asset_uuid"`
	Type              string `json:"type"`
	MimeType          string `json:"mime_type"`
	SizeBytes         int64  `json:"size_bytes"`
	Checksum          string `json:"checksum"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
}
type MediaTaskView struct {
	ProviderStatus    int                `json:"upstream_status,omitempty"`
	ProviderTraceID   string             `json:"provider_trace_id,omitempty"`
	ContractVersion   int                `json:"contract_version"`
	TaskUUID          string             `json:"task_uuid"`
	Status            string             `json:"status"`
	Phase             string             `json:"phase"`
	ModelKey          string             `json:"model_key"`
	Capability        string             `json:"capability"`
	Revision          int64              `json:"revision"`
	TraceID           string             `json:"trace_id"`
	ErrorCode         string             `json:"error_code,omitempty"`
	ProviderErrorCode string             `json:"provider_error_code,omitempty"`
	CancelStatus      string             `json:"cancel_status"`
	Assets            []MediaAssetResult `json:"assets"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	CompletedAt       *time.Time         `json:"completed_at,omitempty"`
}
type MediaSubmitInput struct {
	ModelKey       string         `json:"model_key"`
	Capability     string         `json:"capability"`
	Inputs         []ContentItem  `json:"inputs"`
	Params         map[string]any `json:"params"`
	IdempotencyKey string         `json:"idempotency_key"`
}
type mediaTaskRequest struct {
	MediaSubmitInput
	EffectiveParams map[string]any         `json:"effective_params"`
	Operation       catalog.MediaOperation `json:"operation"`
}
type mediaTaskMessage struct {
	Tenant string `json:"tenant_uuid"`
	Task   string `json:"task_uuid"`
}

type MediaRuntime struct {
	namespace     string
	audit         auditsvc.Service
	providerDelay time.Duration
	ai            *Service
	repo          *r.MediaTaskRepository
	media         *mediasvc.MediaService
	queue         *taskqueue.StreamDriver
	changed       func(context.Context, string, string)
	// 仅用于显式依赖注入和驱动契约测试，生产由正式 factory 构造。
	video func(string) (contract.AsyncVideoClient, error)
	fetch MediaFetcher
}

func NewMediaRuntime(db *gorm.DB, media *mediasvc.MediaService) *MediaRuntime {
	if db == nil {
		return nil
	}
	return &MediaRuntime{providerDelay: 5 * time.Second, ai: NewService(db), repo: r.NewMediaTaskRepository(db), media: media, fetch: newMediaFetcher(), video: func(driver string) (contract.AsyncVideoClient, error) {
		client, err := videofactory.NewClient(driver)
		if err != nil {
			return nil, err
		}
		async, ok := client.(contract.AsyncVideoClient)
		if !ok {
			return nil, MediaError(422, "AI_ASYNC_UNSUPPORTED")
		}
		return async, nil
	}}
}
func (s *MediaRuntime) Configure(queue *taskqueue.StreamDriver, changed func(context.Context, string, string), namespace ...string) {
	if len(namespace) > 0 {
		s.namespace = namespace[0]
	}
	s.queue = queue
	s.changed = changed
}
func MediaTaskEventChannel(tenant, task string) string {
	return "powerx:ai:media:" + tenant + ":" + task
}
func (s *MediaRuntime) notify(ctx context.Context, tenant, task string) {
	if s.changed != nil {
		s.changed(ctx, tenant, task)
	}
}
func mediaTaskView(row *m.MediaTask) *MediaTaskView {
	assets := []MediaAssetResult{}
	if len(row.Result) > 0 {
		_ = json.Unmarshal(row.Result, &assets)
	}
	return &MediaTaskView{ContractVersion: 2, TaskUUID: row.UUID.String(), Status: row.Status, Phase: row.Phase, ModelKey: row.ModelKey, Capability: row.Capability, Revision: row.Revision, TraceID: row.TraceID, ErrorCode: row.ErrorCode, ProviderErrorCode: row.ProviderErrorCode, ProviderStatus: row.ProviderStatus, ProviderTraceID: row.ProviderTraceID, CancelStatus: row.CancelStatus, Assets: assets, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, CompletedAt: row.CompletedAt}
}
func terminalMediaTask(status string) bool {
	return status == "succeeded" || status == "failed" || status == "cancelled"
}

var mediaIdempotency = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

func (s *MediaRuntime) Submit(ctx context.Context, in MediaSubmitInput) (*MediaTaskView, error) {
	if s == nil || s.ai == nil || s.queue == nil {
		return nil, MediaError(503, "AI_TASKBUS_UNAVAILABLE")
	}
	tenant := reqctx.GetTenantUUID(ctx)
	ability := catalog.MediaCapability(in.Capability)
	if ability == catalog.VisionUnderstanding || InvokeCapability(ability) == "" {
		return nil, MediaError(422, "AI_CAPABILITY_INVALID")
	}
	subject, err := s.ai.AuthorizeMedia(ctx, tenant, InvokeCapability(ability))
	if err != nil {
		return nil, err
	}
	if !mediaIdempotency.MatchString(in.IdempotencyKey) {
		return nil, MediaError(400, "AI_IDEMPOTENCY_KEY_REQUIRED")
	}
	in.Capability = ability
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	provider, modelID := splitModelKey(in.ModelKey)
	if canonical := s.ai.mediaRegistry().CanonicalProvider(provider); canonical != "" {
		in.ModelKey = canonical + "/" + modelID
	}
	if existing, e := s.repo.FindRequest(ctx, tenant, subject, ability, in.IdempotencyKey); e == nil {
		raw, _ := json.Marshal(in)
		digest := sha256.Sum256(raw)
		if existing.RequestDigest != hex.EncodeToString(digest[:]) {
			return nil, MediaError(409, "AI_IDEMPOTENCY_CONFLICT")
		}
		if !terminalMediaTask(existing.Status) {
			payload, _ := json.Marshal(mediaTaskMessage{tenant, existing.UUID.String()})
			msg := event_bus.TaskMessage{ID: existing.UUID.String(), TenantKey: mediaQueueTenant, SubscriberID: mediaSubscriber, Topic: MediaTaskChangedTopic, TraceID: existing.TraceID, Payload: payload, Metadata: map[string]string{"taskbus_record_tenant": tenant, "kind": "ai_media_generation"}}
			if e = s.queue.Enqueue(ctx, msg); e != nil {
				failure := MediaError(503, "AI_TASK_ENQUEUE_PENDING")
				failure.Details = map[string]any{"task_uuid": existing.UUID.String()}
				return nil, failure
			}
		}
		return mediaTaskView(existing), nil
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	env, ok, err := s.ai.ResolveTenantEnv(ctx, tenant)
	if err != nil || !ok || env == "" {
		return nil, MediaError(409, "AI_ENV_NOT_CONFIGURED")
	}
	model, _, effective, err := s.ai.prepareMedia(ctx, env, tenant, in.ModelKey, ability, in.Inputs, in.Params)
	if err != nil {
		return nil, err
	}
	in.ModelKey = model.ModelKey()
	in.Capability = ability
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	request, err := json.Marshal(mediaTaskRequest{in, effective, model.Model.Operations[ability]})
	requestLimit := 16 << 20
	if ability == catalog.ImageGeneration {
		requestLimit = contract.MediaRequestMaxBytes
	}
	if err != nil || len(request) > requestLimit {
		return nil, MediaError(422, "AI_INPUT_LIMIT_EXCEEDED")
	}
	trace := reqctx.GetTraceID(ctx)
	if trace == "" {
		trace = uuid.NewString()
	}
	row := &m.MediaTask{TenantUUID: tenant, Subject: subject, Capability: ability, IdempotencyKey: in.IdempotencyKey, RequestDigest: hex.EncodeToString(sum[:]), Env: env, ModelKey: in.ModelKey, Status: "queued", Phase: "queued", Revision: 1, TraceID: trace, Request: datatypes.JSON(request), CancelStatus: "none"}
	var saved *m.MediaTask
	createdTask := false
	var msg event_bus.TaskMessage
	err = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		var created bool
		var e error
		saved, created, e = s.repo.Create(ctx, tx, row)
		if e != nil {
			return e
		}
		payload, _ := json.Marshal(mediaTaskMessage{tenant, saved.UUID.String()})
		msg = event_bus.TaskMessage{ID: saved.UUID.String(), TenantKey: mediaQueueTenant, SubscriberID: mediaSubscriber, Topic: MediaTaskChangedTopic, TraceID: trace, Payload: payload, Metadata: map[string]string{"taskbus_record_tenant": tenant, "kind": "ai_media_generation"}}
		createdTask = created
		if created {
			return s.queue.Register(ctx, tx, msg)
		}
		return nil
	})
	if errors.Is(err, r.ErrIdempotencyConflict) {
		return nil, MediaError(409, "AI_IDEMPOTENCY_CONFLICT")
	}
	if err != nil {
		return nil, err
	}
	if createdTask {
		s.auditMedia(ctx, saved, "submit")
	}
	if !terminalMediaTask(saved.Status) {
		_ = s.queue.Enqueue(ctx, msg)
	} // 注册已持久化，Redis 故障由正式 Recover 补投。
	s.notify(ctx, tenant, saved.UUID.String())
	return mediaTaskView(saved), nil
}
func validMediaTaskID(id string) bool {
	u, e := uuid.Parse(id)
	return e == nil && u != uuid.Nil && u.String() == id
}
func (s *MediaRuntime) Get(ctx context.Context, id string) (*MediaTaskView, error) {
	if s == nil {
		return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	if !validMediaTaskID(id) {
		return nil, MediaError(400, "AI_TASK_UUID_INVALID")
	}
	tenant := reqctx.GetTenantUUID(ctx)
	subject, err := s.ai.AuthorizeMedia(ctx, tenant, MediaTaskReadCapability)
	if err != nil {
		return nil, err
	}
	row, err := s.repo.Get(ctx, tenant, id, subject)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, MediaError(404, "AI_TASK_NOT_FOUND")
	}
	if err != nil {
		return nil, err
	}
	return mediaTaskView(row), nil
}
func (s *MediaRuntime) AuthorizeRead(ctx context.Context) error {
	if s == nil {
		return MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	_, err := s.ai.AuthorizeMedia(ctx, reqctx.GetTenantUUID(ctx), MediaTaskReadCapability)
	return err
}
func (s *MediaRuntime) Cancel(ctx context.Context, id string) (*MediaTaskView, error) {
	if s == nil {
		return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	if !validMediaTaskID(id) {
		return nil, MediaError(400, "AI_TASK_UUID_INVALID")
	}
	tenant := reqctx.GetTenantUUID(ctx)
	subject, err := s.ai.AuthorizeMedia(ctx, tenant, MediaTaskCancelCapability)
	if err != nil {
		return nil, err
	}
	var saved *m.MediaTask
	err = s.repo.Update(ctx, nil, tenant, id, func(row *m.MediaTask) error {
		if row.Subject != subject {
			return gorm.ErrRecordNotFound
		}
		if terminalMediaTask(row.Status) {
			saved = row
			return nil
		}
		if row.Phase == "queued" {
			row.Status = "cancelled"
			row.CancelStatus = "confirmed"
			row.Phase = "completed"
			now := time.Now().UTC()
			row.CompletedAt = &now
		} else if row.CancelStatus == "none" {
			row.CancelStatus = "requested"
		}
		saved = row
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, MediaError(404, "AI_TASK_NOT_FOUND")
	}
	if err != nil {
		return nil, err
	}
	s.auditMedia(ctx, saved, "cancel")
	s.notify(ctx, tenant, id)
	return mediaTaskView(saved), nil
}

func (s *MediaRuntime) resolveTask(ctx context.Context, row *m.MediaTask) (*mediaTaskRequest, *catalog.MediaModel, *aiconfig.ModelConfig, error) {
	var request mediaTaskRequest
	if json.Unmarshal(row.Request, &request) != nil {
		return nil, nil, nil, MediaError(500, "AI_TASK_PAYLOAD_INVALID")
	}
	model, err := s.ai.mediaModel(ctx, row.Env, row.TenantUUID, row.ModelKey, row.Capability)
	if err != nil {
		return nil, nil, nil, err
	}
	mc, _, err := s.ai.mediaConfig(ctx, row.Env, row.TenantUUID, model, row.Capability)
	return &request, model, mc, err
}

func (s *MediaRuntime) EventChannel(tenant, task string) string {
	return redisnamespace.Key(s.namespace, MediaTaskEventChannel(tenant, task))
}
