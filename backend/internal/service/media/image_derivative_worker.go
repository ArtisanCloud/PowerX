package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const derivativeQueueTenant = "core-media"
const derivativeSubscriber = "image-derivatives-v1"
const derivativeGroup = "media-workers"
const ImageDerivativeChangedTopic = "media.image-derivatives.changed"

type derivativeTask struct {
	Tenant         string `json:"tenant_uuid"`
	Asset          string `json:"asset_uuid"`
	TaskID         string `json:"task_id"`
	Recipe         string `json:"recipe"`
	SourceChecksum string `json:"source_checksum"`
}

// ConfigureDerivativeQueue 注入统一 TaskBus 驱动，Media 不自行创建队列或任务历史表。
func (s *MediaService) ConfigureDerivativeQueue(queue *taskqueue.StreamDriver) {
	s.derivativeQueue = queue
}
func (s *MediaService) RunImageDerivativeWorker(ctx context.Context) error {
	if s.derivativeQueue == nil || s.derivatives == nil {
		return errors.New("media TaskBus unavailable")
	}
	worker := "media-" + uuid.NewString()
	for {
		if e := s.derivativeQueue.Recover(ctx, derivativeQueueTenant, derivativeSubscriber); e == nil {
			break
		} else {
			logger.WarnF(ctx, "TaskBus recovery: %v", e)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	for ctx.Err() == nil {
		deliveries, e := s.derivativeQueue.Dequeue(ctx, derivativeQueueTenant, derivativeSubscriber, derivativeGroup, worker, 1, 3*time.Second)
		if e != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.WarnF(ctx, "media TaskBus dequeue: %v", e)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		}
		for _, d := range deliveries {
			if e := s.handleDerivativeDelivery(ctx, d); e != nil {
				logger.WarnF(ctx, "media TaskBus delivery: %v", e)
			}
		}
	}
	return ctx.Err()
}

var errDerivativeStale = errors.New("media derivative source superseded")
var errDerivativeVariantPending = errors.New("variant upload still pending")

func (s *MediaService) handleDerivativeDelivery(ctx context.Context, d event_bus.StreamTaskDelivery) error {
	var task derivativeTask
	if json.Unmarshal(d.Message.Payload, &task) != nil || task.Tenant == "" || task.Asset == "" || task.TaskID == "" {
		return s.derivativeQueue.Reject(ctx, d, "TASKBUS_INVALID_PAYLOAD", true)
	}
	s.publishDerivative(ctx, task.Tenant, task.Asset)
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	err := s.processImageDerivative(runCtx, task, d)
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	defer s.publishDerivative(finishCtx, task.Tenant, task.Asset)
	if errors.Is(err, event_bus.ErrLeaseStale) {
		return err
	}
	if errors.Is(err, taskqueue.ErrTaskTerminal) {
		return s.derivativeQueue.Ack(finishCtx, d)
	}
	if errors.Is(err, errDerivativeStale) || errors.Is(err, gorm.ErrRecordNotFound) {
		return s.derivativeQueue.Cancel(finishCtx, d, "source_superseded")
	}
	if errors.Is(err, errDerivativeVariantPending) {
		return s.derivativeQueue.Defer(finishCtx, d, time.Now().Add(5*time.Second), "variant_upload_pending")
	}
	if err != nil {
		var render *imageRenderError
		return s.derivativeQueue.Reject(finishCtx, d, derivativeErrorCode(err), errors.As(err, &render))
	}
	return s.derivativeQueue.Ack(finishCtx, d)
}
func (s *MediaService) processImageDerivative(ctx context.Context, task derivativeTask, delivery event_bus.StreamTaskDelivery) error {
	var source *m.MediaAsset
	err := s.derivatives.WithAsset(ctx, task.Tenant, task.Asset, func(st *r.DerivativeState) error {
		ref := derivativeRef(st.Asset)
		if ref.TaskID != task.TaskID || ref.Recipe != task.Recipe || task.Recipe != ImageDerivativeRecipe || st.Asset.UploadState != m.UploadStateReady || st.Asset.ExpectedChecksum != task.SourceChecksum {
			return errDerivativeStale
		}
		for _, v := range st.Variants {
			if (v.Variant == "preview" || v.Variant == "thumbnail") && (v.UploadState == m.UploadStatePending || v.UploadState == m.UploadStateUploaded) && v.UploadExpiresAt != nil && time.Now().Before(*v.UploadExpiresAt) {
				return errDerivativeVariantPending
			}
		}
		copy := *st.Asset
		source = &copy
		return nil
	})
	if err != nil {
		return err
	}
	return s.generateImageVariants(ctx, source, task, delivery)
}

func (s *MediaService) generateImageVariants(ctx context.Context, a *m.MediaAsset, task derivativeTask, delivery event_bus.StreamTaskDelivery) error {
	objectVersion := uuid.NewString()
	object, err := s.manager.Get(ctx, a.Driver, driver.GetObjectInput{Bucket: a.Bucket, ObjectKey: a.StorageKey})
	if err != nil {
		return err
	}
	if object == nil || object.Body == nil {
		return errors.New("original object missing")
	}
	defer object.Body.Close()
	img, format, err := renderSource(ctx, object.Body, a.ExpectedChecksum)
	if err != nil {
		return err
	}
	generated := make(map[string]*m.MediaAssetVariant, 2)
	defer func() { // 未被事务接纳的对象不得遗留（已就绪旧对象由正常 purge 生命周期管理）。
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		for _, v := range generated {
			_ = s.manager.Delete(cleanupCtx, v.Driver, driver.DeleteObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey})
		}
	}()
	for _, p := range imageProfiles {
		raw, mime, w, h, e := encodeDerivative(ctx, img, format, p.Edge)
		if e != nil {
			return e
		}
		sum := sha256.Sum256(raw)
		checksum := hex.EncodeToString(sum[:])
		key := fmt.Sprintf("%s/generated/%s/%s/%s/%s", a.UUID.String(), a.TenantUUID, ImageDerivativeRecipe, objectVersion, p.Name)
		v := &m.MediaAssetVariant{TenantUUID: a.TenantUUID, AssetUUID: a.UUID.String(), Variant: p.Name, Name: p.Name, Driver: a.Driver, Bucket: a.Bucket, BaseURL: a.BaseURL, StorageKey: key, SizeBytes: int64(len(raw)), MimeType: mime, UploadState: m.UploadStateReady, ExpectedChecksum: checksum, TicketVersion: 1, Meta: derivativeMeta(a, checksum, w, h)}
		v.UUID = uuid.New()
		now := time.Now().UTC()
		v.CompletedAt = &now
		generated[p.Name] = v
		if _, e = s.manager.Put(ctx, a.Driver, driver.PutObjectInput{Bucket: a.Bucket, ObjectKey: key, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: mime, Overwrite: false}); e != nil {
			return e
		}
	}
	accepted := []string{}
	obsolete := []m.MediaAssetVariant{}
	err = s.derivativeQueue.Complete(ctx, delivery, func(tx *gorm.DB) error {
		return s.derivatives.WithTx(ctx, tx, a.TenantUUID, a.UUID.String(), func(st *r.DerivativeState) error {
			ref := derivativeRef(st.Asset)
			if ref.TaskID != task.TaskID || st.Asset.UploadState != m.UploadStateReady || st.Asset.ExpectedChecksum != task.SourceChecksum {
				return errDerivativeStale
			}
			for _, p := range imageProfiles {
				var old *m.MediaAssetVariant
				for _, v := range st.Variants {
					if v.Variant == p.Name {
						old = v
						break
					}
				}
				if old != nil {
					ok, e := s.usableImageVariant(ctx, st.Asset, old)
					if e != nil {
						return e
					}
					if ok {
						continue
					}
					if (old.UploadState == m.UploadStatePending || old.UploadState == m.UploadStateUploaded) && old.UploadExpiresAt != nil && time.Now().Before(*old.UploadExpiresAt) {
						return errDerivativeVariantPending
					}
					// 不覆盖调用方仍存在的变体；缺失对象和 Core 生成的旧版本可补齐。
					v := generated[p.Name]
					if stringFromMeta(extractMeta(old.Meta), "generator") == "powerx-media" {
						obsolete = append(obsolete, *old)
					}
					v.PowerUUIDModel = old.PowerUUIDModel
					v.TicketVersion = old.TicketVersion + 1
					*old = *v
				} else {
					st.Variants = append(st.Variants, generated[p.Name])
				}
				accepted = append(accepted, p.Name)
			}
			ref.Missing = nil
			setDerivativeRef(st.Asset, ref)
			return nil
		})
	})
	if err == nil {
		for _, name := range accepted {
			delete(generated, name)
		}
		for _, v := range obsolete {
			if e := s.manager.Delete(ctx, v.Driver, driver.DeleteObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey, Force: true}); e != nil {
				logger.WarnF(ctx, "media obsolete derivative cleanup: %v", e)
			}
		}
	}
	return err
}
