package media

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const ImageDerivativeRecipe = "standard-image-v1"

func ImageDerivativeEventChannel(tenant, asset string) string {
	return "powerx:media:derivatives:" + tenant + ":" + asset
}

var imageProfiles = []struct {
	Name string
	Edge int
}{{"preview", 1600}, {"thumbnail", 320}}

type derivativeRepository interface {
	WithAsset(context.Context, string, string, func(*r.DerivativeState) error) error
	WithTx(context.Context, *gorm.DB, string, string, func(*r.DerivativeState) error) error
}

// derivativeReference 只关联业务对象与统一任务，不保存重试、状态、租约或执行 token。
type derivativeReference struct {
	TaskID         string   `json:"task_id"`
	Recipe         string   `json:"recipe"`
	SourceChecksum string   `json:"source_checksum"`
	Missing        []string `json:"missing_variants"`
}

func derivativeRef(a *m.MediaAsset) derivativeReference {
	var out derivativeReference
	meta := extractMeta(a.Meta)
	raw, _ := json.Marshal(meta["image_derivatives"])
	_ = json.Unmarshal(raw, &out)
	return out
}
func setDerivativeRef(a *m.MediaAsset, ref derivativeReference) {
	meta := extractMeta(a.Meta)
	meta["image_derivatives"] = ref
	raw, _ := json.Marshal(meta)
	a.Meta = raw
}

// ImageDerivatives 将原图上传与派生处理分开，任务状态投影自统一 TaskBus 历史。
type ImageDerivatives struct {
	AssetUUID   string                `json:"asset_uuid"`
	UploadState string                `json:"upload_state"`
	Status      string                `json:"status"`
	Recipe      string                `json:"recipe"`
	JobUUID     string                `json:"job_uuid,omitempty"`
	Attempts    int                   `json:"attempts"`
	ErrorCode   string                `json:"error_code,omitempty"`
	Retryable   bool                  `json:"retryable"`
	Items       []ImageDerivativeItem `json:"items"`
}
type ImageDerivativeItem struct {
	Variant     string `json:"variant"`
	Status      string `json:"status"`
	VariantUUID string `json:"variant_uuid,omitempty"`
}

func imageMIME(mime string) bool { return strings.HasPrefix(normalizeUploadMIME(mime), "image/") }
func supportedImageMIME(mime string) bool {
	switch normalizeUploadMIME(mime) {
	case "image/jpeg", "image/png", "image/gif":
		return true
	}
	return false
}
func derivativeView(st *r.DerivativeState, history *ef.TaskHistory) *ImageDerivatives {
	a := st.Asset
	ref := derivativeRef(a)
	out := &ImageDerivatives{AssetUUID: a.UUID.String(), UploadState: a.UploadState, Status: "not_requested", Recipe: ImageDerivativeRecipe, JobUUID: ref.TaskID, Items: make([]ImageDerivativeItem, 0, 2)}
	if history != nil {
		out.Attempts = history.Attempt
		out.ErrorCode = history.ErrorMessage
		switch history.Status {
		case ef.TaskHistoryStatusPending, ef.TaskHistoryStatusDeferred:
			out.Status = "queued"
		case ef.TaskHistoryStatusRunning:
			out.Status = "processing"
		case ef.TaskHistoryStatusCompleted:
			out.Status = "ready"
		case ef.TaskHistoryStatusFailed, ef.TaskHistoryStatusCancelled:
			out.Status = "failed"
		}
		if history.ErrorMessage == "TASKBUS_ENQUEUE_FAILED" {
			out.Status = "scheduling_failed"
		}
		out.Retryable = out.Status == "failed" || out.Status == "scheduling_failed"
	}
	if a.UploadState != m.UploadStateReady {
		out.Status = "waiting_upload"
	}
	if !supportedImageMIME(a.MimeType) || stringFromMeta(extractMeta(a.Meta), "upload_method") == string(UploadMethodExternalLink) {
		out.Status = "unsupported"
	}
	for _, p := range imageProfiles {
		item := ImageDerivativeItem{Variant: p.Name, Status: out.Status}
		for _, v := range st.Variants {
			if v.Variant == p.Name && v.UploadState == m.UploadStateReady {
				item.Status = "ready"
				item.VariantUUID = v.UUID.String()
			}
		}
		for _, missing := range ref.Missing {
			if missing == p.Name && out.Status != "ready" {
				item.Status = out.Status
				item.VariantUUID = ""
			}
		}
		out.Items = append(out.Items, item)
	}
	if len(ref.Missing) == 0 && out.Items[0].Status == "ready" && out.Items[1].Status == "ready" {
		out.Status = "ready"
		out.ErrorCode = ""
		out.Retryable = false
	}
	return out
}
func (s *MediaService) derivativeHistory(ctx context.Context, st *r.DerivativeState) (*ef.TaskHistory, error) {
	ref := derivativeRef(st.Asset)
	if ref.TaskID == "" {
		return nil, nil
	}
	if s.derivativeQueue == nil {
		return nil, MediaUpstreamDependencyError(errors.New("TaskBus unavailable"))
	}
	return s.derivativeQueue.StatusWithTx(ctx, st.Tx, st.Asset.TenantUUID, derivativeSubscriber, ref.TaskID)
}
func (s *MediaService) ImageDerivativeStatus(ctx context.Context, tenant, asset string) (*ImageDerivatives, error) {
	if s.derivatives == nil {
		return nil, MediaUpstreamDependencyError(errors.New("media persistence unavailable"))
	}
	var out *ImageDerivatives
	err := s.derivatives.WithAsset(ctx, tenant, asset, func(st *r.DerivativeState) error {
		history, e := s.derivativeHistory(ctx, st)
		if e != nil {
			return e
		}
		out = derivativeView(st, history)
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAssetNotFound
	}
	return out, err
}

// EnsureImageDerivatives 业务检查与共享任务登记同事务；投递、恢复、重试归 TaskBus。
func (s *MediaService) EnsureImageDerivatives(ctx context.Context, tenant, asset string, retryFailed bool) (*ImageDerivatives, error) {
	if s.derivatives == nil || s.manager == nil || s.derivativeQueue == nil {
		return nil, MediaUpstreamDependencyError(errors.New("media TaskBus unavailable"))
	}
	var out *ImageDerivatives
	var scheduled *event_bus.TaskMessage
	err := s.derivatives.WithAsset(ctx, tenant, asset, func(st *r.DerivativeState) error {
		history, e := s.derivativeHistory(ctx, st)
		if e != nil {
			return e
		}
		out = derivativeView(st, history)
		if out.Status == "waiting_upload" || out.Status == "unsupported" {
			return nil
		}
		a := st.Asset
		ref := derivativeRef(a)
		if _, e := s.manager.Stat(ctx, a.Driver, driver.StatObjectInput{Bucket: a.Bucket, ObjectKey: a.StorageKey}); e != nil {
			out.Status = "waiting_source"
			out.ErrorCode = "MEDIA_IMAGE_SOURCE_UNAVAILABLE"
			out.Retryable = true
			return nil
		}
		missing := []string{}
		for _, p := range imageProfiles {
			valid := false
			for _, v := range st.Variants {
				if v.Variant == p.Name {
					valid, e = s.usableImageVariant(ctx, a, v)
					if e != nil {
						return e
					}
				}
			}
			if !valid {
				missing = append(missing, p.Name)
			}
		}
		if len(missing) == 0 {
			ref.Missing = nil
			setDerivativeRef(a, ref)
			out = derivativeView(st, history)
			out.Status = "ready"
			return nil
		}
		if history != nil && ref.Recipe == ImageDerivativeRecipe && ref.SourceChecksum == a.ExpectedChecksum {
			if history.Status == ef.TaskHistoryStatusPending || history.Status == ef.TaskHistoryStatusDeferred || history.Status == ef.TaskHistoryStatusRunning {
				var msg event_bus.TaskMessage
				if e := json.Unmarshal([]byte(history.Payload), &msg); e != nil {
					return e
				}
				scheduled = &msg
				return nil
			}
			if history.Status == ef.TaskHistoryStatusFailed && !retryFailed {
				return nil
			}
		}
		ref = derivativeReference{TaskID: uuid.NewString(), Recipe: ImageDerivativeRecipe, SourceChecksum: a.ExpectedChecksum, Missing: missing}
		task := derivativeTask{Tenant: tenant, Asset: asset, TaskID: ref.TaskID, Recipe: ref.Recipe, SourceChecksum: ref.SourceChecksum}
		raw, _ := json.Marshal(task)
		msg := event_bus.TaskMessage{ID: ref.TaskID, TenantKey: derivativeQueueTenant, SubscriberID: derivativeSubscriber, Topic: ImageDerivativeChangedTopic, Payload: raw, Metadata: map[string]string{"taskbus_record_tenant": tenant, "kind": "media_image_derivatives"}}
		if e := s.derivativeQueue.Register(ctx, st.Tx, msg); e != nil {
			return e
		}
		setDerivativeRef(a, ref)
		scheduled = &msg
		out = derivativeView(st, nil)
		out.Status = "queued"
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAssetNotFound
	}
	if err != nil {
		return nil, err
	}
	if scheduled != nil {
		enqueueErr := s.derivativeQueue.Enqueue(ctx, *scheduled)
		state, e := s.ImageDerivativeStatus(ctx, tenant, asset)
		if e != nil {
			return nil, e
		}
		out = state
		if enqueueErr != nil && out.Status != "scheduling_failed" {
			return nil, enqueueErr
		}
		s.publishDerivative(ctx, tenant, asset)
	}
	return out, nil
}

func (s *MediaService) usableImageVariant(ctx context.Context, a *m.MediaAsset, v *m.MediaAssetVariant) (bool, error) {
	if v.UploadState != m.UploadStateReady || !imageMIME(v.MimeType) {
		return false, nil
	}
	meta := extractMeta(v.Meta)
	if stringFromMeta(meta, "generator") == "powerx-media" && (stringFromMeta(meta, "recipe") != ImageDerivativeRecipe || stringFromMeta(meta, "source_checksum") != a.ExpectedChecksum) {
		return false, nil
	}
	stat, err := s.manager.Stat(ctx, v.Driver, driver.StatObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey})
	if errors.Is(err, driver.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return stat != nil && stat.Size > 0 && stat.Size == v.SizeBytes, nil
}

// 旧客户端可能仍尝试上传其本地预览。Core 已生成的同名标准变体是可复用结果，
// 不要求该客户端碰巧采用同一编码器；客户端应尊重返回的 ready/MIME/size。
// 插件自行上传的变体仍严格执行原 checksum/size/MIME 冲突与重试规则。
func (s *MediaService) reuseOrRetryVariant(ctx context.Context, parent *m.MediaAsset, v *m.MediaAssetVariant, in CreateAssetVariantInput) (*AssetVariant, error) {
	if stringFromMeta(extractMeta(v.Meta), "generator") == "powerx-media" {
		if in.Checksum != "" {
			if checksum, err := contentSHA256FromMetadata(map[string]any{"content_sha256": in.Checksum}); err != nil || checksum == "" {
				return nil, ErrContentSHA256Invalid
			}
		}
		if ok, err := s.usableImageVariant(ctx, parent, v); err != nil {
			return nil, err
		} else if ok {
			return toAssetVariant(v), nil
		}
	}
	return s.retryVariantUpload(ctx, v, in)
}

func (s *MediaService) withImageDerivatives(ctx context.Context, a *Asset, ensure bool) *Asset {
	if a == nil || !imageMIME(a.MimeType) {
		return a
	}
	var state *ImageDerivatives
	var err error
	if ensure {
		state, err = s.EnsureImageDerivatives(ctx, a.TenantUUID, a.UUID, false)
	} else {
		state, err = s.ImageDerivativeStatus(ctx, a.TenantUUID, a.UUID)
	}
	if err != nil {
		state = &ImageDerivatives{AssetUUID: a.UUID, UploadState: a.UploadState, Status: "unavailable", Recipe: ImageDerivativeRecipe, ErrorCode: "MEDIA_DERIVATIVE_DEPENDENCY_UNAVAILABLE", Retryable: true, Items: []ImageDerivativeItem{}}
	}
	a.Derivatives = state
	return a
}

func derivativeMeta(a *m.MediaAsset, checksum string, width, height int) []byte {
	raw, _ := json.Marshal(map[string]any{"generator": "powerx-media", "recipe": ImageDerivativeRecipe, "source_checksum": a.ExpectedChecksum, "content_sha256": checksum, "width": width, "height": height})
	return raw
}

func (s *MediaService) publishDerivative(ctx context.Context, tenant, asset string) {
	if s.derivativeChanged != nil {
		s.derivativeChanged(ctx, tenant, asset)
	}
}

// SetDerivativeEvents 提供状态失效通知；消费方收到事件后按已鉴权接口读取状态。
func (s *MediaService) SetDerivativeEvents(changed func(context.Context, string, string)) {
	s.derivativeChanged = changed
}
