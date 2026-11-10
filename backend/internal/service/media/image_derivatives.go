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

const ImageDerivativeRecipe = "standard-image-v2"
const legacyImageDerivativeRecipe = "standard-image-v1"
const Thumbnail64MaxBytes = 32 << 10

type imageProfile struct {
	Name string
	Edge int
}

func recipeProfiles(recipe string) []imageProfile {
	if recipe == legacyImageDerivativeRecipe {
		return imageProfiles[:2]
	}
	if recipe == ImageDerivativeRecipe {
		return imageProfiles
	}
	return nil
}

func recipeSubscriber(recipe string) string {
	if recipe == legacyImageDerivativeRecipe {
		return legacyDerivativeSubscriber
	}
	return derivativeSubscriber
}

func ImageDerivativeEventChannel(tenant, asset string) string {
	return "powerx:media:derivatives:" + tenant + ":" + asset
}

var imageProfiles = []imageProfile{{"preview", 1600}, {"thumbnail", 320}, {"thumbnail_64", 64}}

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
	ErrorCode   string `json:"error_code,omitempty"`
	Retryable   bool   `json:"retryable,omitempty"`
}

func imageMIME(mime string) bool { return strings.HasPrefix(normalizeUploadMIME(mime), "image/") }
func supportedImageMIME(mime string) bool {
	switch normalizeUploadMIME(mime) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

// variantRecordUsable is a read-only projection. Object existence is checked by
// ensure/worker, never by a status GET. Missing objects discovered there remain
// unavailable until an accepted result clears ref.Missing.
func variantRecordUsable(a *m.MediaAsset, v *m.MediaAssetVariant) bool {
	if v.UploadState != m.UploadStateReady || !imageMIME(v.MimeType) || v.UUID == uuid.Nil || v.SizeBytes <= 0 {
		return false
	}
	meta := extractMeta(v.Meta)
	if stringFromMeta(meta, "generator") == "powerx-media" {
		compatible := false
		edge := 0
		for _, p := range recipeProfiles(stringFromMeta(meta, "recipe")) {
			if p.Name == v.Variant {
				compatible = true
				edge = p.Edge
				break
			}
		}
		w, _ := meta["width"].(float64)
		h, _ := meta["height"].(float64)
		if a.ExpectedChecksum == "" || !compatible || w < 1 || h < 1 || w > float64(edge) || h > float64(edge) || v.SizeBytes < 1 || stringFromMeta(meta, "source_checksum") != a.ExpectedChecksum {
			return false
		}
	}
	if v.Variant == "thumbnail_64" {
		w, _ := meta["width"].(float64)
		h, _ := meta["height"].(float64)
		if w < 1 || h < 1 || w > 64 || h > 64 || v.SizeBytes < 1 || v.SizeBytes > Thumbnail64MaxBytes ||
			(normalizeUploadMIME(v.MimeType) != "image/png" && normalizeUploadMIME(v.MimeType) != "image/jpeg") ||
			stringFromMeta(meta, "source_checksum") != a.ExpectedChecksum {
			return false
		}
		if stringFromMeta(meta, "generator") != "powerx-media" && meta["validated_profile"] != "thumbnail_64" {
			return false
		}
	}
	return true
}

func derivativeView(st *r.DerivativeState, history *ef.TaskHistory) *ImageDerivatives {
	a := st.Asset
	ref := derivativeRef(a)
	out := &ImageDerivatives{AssetUUID: a.UUID.String(), UploadState: a.UploadState, Status: "not_requested", Recipe: ImageDerivativeRecipe, JobUUID: ref.TaskID, Items: make([]ImageDerivativeItem, 0, len(imageProfiles))}
	if history != nil {
		out.Attempts = history.Attempt
		out.ErrorCode = history.ErrorMessage
		switch history.Status {
		case ef.TaskHistoryStatusPending, ef.TaskHistoryStatusDeferred:
			out.Status = "queued"
		case ef.TaskHistoryStatusRunning:
			out.Status = "processing"
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
	allReady := true
	for _, p := range imageProfiles {
		item := ImageDerivativeItem{Variant: p.Name, Status: out.Status}
		// A completed legacy recipe has not requested the new profile.
		if ref.Recipe == legacyImageDerivativeRecipe && p.Name == "thumbnail_64" {
			item.Status = "not_requested"
		}
		missing := false
		for _, name := range ref.Missing {
			if name == p.Name {
				missing = true
			}
		}
		for _, v := range st.Variants {
			acceptedAfterRequest := history != nil && history.SubmittedAt != nil && v.CompletedAt != nil && !v.CompletedAt.Before(*history.SubmittedAt)
			if v.Variant == p.Name && (!missing || acceptedAfterRequest) && variantRecordUsable(a, v) {
				item.Status, item.VariantUUID = "ready", v.UUID.String()
				break
			}
		}
		if item.Status != "ready" {
			allReady = false
			item.ErrorCode, item.Retryable = out.ErrorCode, out.Retryable
		}
		out.Items = append(out.Items, item)
	}
	if !allReady && history != nil && history.Status == ef.TaskHistoryStatusCompleted && ref.Recipe == ImageDerivativeRecipe && out.Status == "not_requested" {
		out.Status, out.ErrorCode, out.Retryable = "unavailable", "MEDIA_DERIVATIVE_RESULT_INCOMPLETE", true
		for i := range out.Items {
			if out.Items[i].Status != "ready" {
				out.Items[i].Status, out.Items[i].ErrorCode, out.Items[i].Retryable = out.Status, out.ErrorCode, true
			}
		}
	}
	if allReady && out.Status != "waiting_upload" && out.Status != "unsupported" {
		out.Status, out.ErrorCode, out.Retryable = "ready", "", false
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
	return s.derivativeQueue.StatusWithTx(ctx, st.Tx, st.Asset.TenantUUID, recipeSubscriber(ref.Recipe), ref.TaskID)
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
	return s.ensureImageDerivatives(ctx, tenant, asset, retryFailed, nil)
}

func (s *MediaService) ensureImageDerivatives(ctx context.Context, tenant, asset string, retryFailed bool, expected *ImageDerivativeBackfillExpectation) (*ImageDerivatives, error) {
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
			for i := range out.Items {
				if out.Items[i].Status != "ready" {
					out.Items[i].Status, out.Items[i].ErrorCode, out.Items[i].Retryable = out.Status, out.ErrorCode, true
				}
			}
			return nil
		}
		if a.ExpectedChecksum == "" {
			checksum, e := s.storedImageChecksum(ctx, a)
			if e != nil {
				return e
			}
			a.ExpectedChecksum = checksum
			meta := extractMeta(a.Meta)
			meta["content_sha256"] = checksum
			a.Meta, _ = json.Marshal(meta)
		}
		if expected != nil && (expected.TenantUUID != a.TenantUUID || expected.SourceChecksum != a.ExpectedChecksum) {
			return errBackfillPlanChanged
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
		// Recheck under the parent lock: a disappeared preview after scan must not
		// silently expand a reviewed thumbnail-only plan into a full regeneration.
		if expected != nil && !expected.allows(missing) {
			return errBackfillPlanChanged
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
		scheduled, e = s.registerDerivativeTask(ctx, st, ImageDerivativeRecipe, missing)
		if e != nil {
			return e
		}
		out = derivativeView(st, &ef.TaskHistory{Status: ef.TaskHistoryStatusPending})
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

// registerDerivativeTask participates in the parent's existing transaction.
// Legacy completion uses it to persist its v2 successor atomically, so a crash
// between task completion and Redis enqueue cannot lose the required new work.
func (s *MediaService) registerDerivativeTask(ctx context.Context, st *r.DerivativeState, recipe string, missing []string) (*event_bus.TaskMessage, error) {
	a := st.Asset
	ref := derivativeReference{TaskID: uuid.NewString(), Recipe: recipe, SourceChecksum: a.ExpectedChecksum, Missing: missing}
	task := derivativeTask{Tenant: a.TenantUUID, Asset: a.UUID.String(), TaskID: ref.TaskID, Recipe: recipe, SourceChecksum: ref.SourceChecksum}
	raw, _ := json.Marshal(task)
	msg := event_bus.TaskMessage{ID: ref.TaskID, TenantKey: derivativeQueueTenant, SubscriberID: recipeSubscriber(recipe), Topic: ImageDerivativeChangedTopic, Payload: raw, Metadata: map[string]string{"taskbus_record_tenant": a.TenantUUID, "kind": "media_image_derivatives"}}
	if err := s.derivativeQueue.Register(ctx, st.Tx, msg); err != nil {
		return nil, err
	}
	setDerivativeRef(a, ref)
	return &msg, nil
}

func (s *MediaService) usableImageVariant(ctx context.Context, a *m.MediaAsset, v *m.MediaAssetVariant) (bool, error) {
	if !variantRecordUsable(a, v) {
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
	if len(state.Items) == 0 && state.Status == "unavailable" {
		for _, p := range imageProfiles {
			state.Items = append(state.Items, ImageDerivativeItem{Variant: p.Name, Status: "unavailable", ErrorCode: state.ErrorCode, Retryable: true})
		}
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

// SetDerivativeEventNamespace must match TaskBus's stable deployment namespace.
func (s *MediaService) SetDerivativeEventNamespace(namespace string) {
	s.derivativeEventNamespace = strings.TrimSpace(namespace)
}
func (s *MediaService) DerivativeEventChannel(tenant, asset string) string {
	return event_bus.NamespacedTaskKey(s.derivativeEventNamespace, ImageDerivativeEventChannel(tenant, asset))
}
