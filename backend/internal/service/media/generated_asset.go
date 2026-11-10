package media

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"io"
)

// StoreGenerated 是 Core worker 的受管成果转存；先完成字节校验再公布就绪资产。
func (s *MediaService) StoreGenerated(ctx context.Context, tenant, task string, index int, kind, mime, checksum string, size int64, body io.ReadSeeker) (*Asset, error) {
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("powerx:ai:%s:%s:%d", tenant, task, index))).String()
	if old, err := s.GetAsset(ctx, tenant, id, false); err == nil && old.UploadState == "ready" {
		if old.SizeBytes != size || old.Metadata["content_sha256"] != checksum {
			return nil, ErrUploadValidationFailed
		}
		_, object, e := s.OpenAssetResource(ctx, tenant, id)
		if e != nil || object == nil || object.Body == nil {
			return nil, ErrUploadValidationFailed
		}
		validation, e := validateUploadedObject(object.Body, object.Size, size, mime, checksum, checksum)
		_ = object.Body.Close()
		if e != nil || validation.Reason != "" {
			return nil, ErrUploadValidationFailed
		}
		return old, nil
	}
	asset, err := s.CreateAsset(ctx, CreateAssetInput{TenantUUID: tenant, Name: task + "-" + fmt.Sprint(index), StorageKey: id, SizeBytes: size, MimeType: mime, OwnerType: "ai_media_task", OwnerID: task, UploadMethod: UploadMethodPresign, ContentSHA256: checksum, Metadata: map[string]any{"ai_task_uuid": task, "kind": kind}})
	if err != nil {
		return nil, err
	}
	if asset.UploadState == "ready" {
		return asset, nil
	}
	if _, err = body.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err = s.PutHostTransfer(ctx, asset, body, size, mime); err != nil {
		return nil, err
	}
	return s.CompleteAssetUpload(ctx, CompleteAssetUploadInput{TenantUUID: tenant, UUID: asset.UUID, Checksum: checksum})
}
