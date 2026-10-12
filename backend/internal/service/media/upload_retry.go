package media

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"gorm.io/datatypes"
)

func uploadMayRetry(state string, expires *time.Time) bool {
	return state == m.UploadStateFailed || ((state == m.UploadStatePending || state == m.UploadStateUploaded) && expires != nil && !time.Now().Before(*expires))
}

// 重复创建是一次受原管理权限保护的新上传尝试。只恢复失败/过期资产，
// 不覆盖已就绪内容、不改变归属或上传约束、不复活已删除资产。
func (s *MediaService) retryAssetUpload(ctx context.Context, existing *m.MediaAsset, in CreateAssetInput, checksum string) (*Asset, error) {
	if !uploadMayRetry(existing.UploadState, existing.UploadExpiresAt) {
		return toAsset(existing), nil
	}
	var result *Asset
	var previous string
	err := s.repo.WithAssetUpload(ctx, strings.TrimSpace(in.TenantUUID), existing.UUID.String(), func(row *m.MediaAsset) error {
		if !uploadMayRetry(row.UploadState, row.UploadExpiresAt) {
			result = toAsset(row)
			return nil
		}
		if row.ExpectedChecksum != checksum || row.SizeBytes != in.SizeBytes || normalizeUploadMIME(row.MimeType) == "" || normalizeUploadMIME(row.MimeType) != normalizeUploadMIME(in.MimeType) || row.UploadTicketVersion == ^uint64(0) {
			return ErrInvalidStatusTransition
		}
		previous = row.UploadState
		expires := time.Now().Add(time.Hour).UTC()
		row.Meta = uploadRetryMetadata(row.Meta, previous, row.UploadTicketVersion)
		row.UploadState, row.UploadExpiresAt = m.UploadStatePending, &expires
		row.UploadTicketVersion++
		if in.OperatorID != nil {
			row.UpdatedBy = in.OperatorID
		}
		result = toAsset(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if previous != "" {
		s.emitAudit(ctx, existing.TenantUUID, "media.asset.upload_retry", existing.UUID.String(), in.OperatorID, map[string]any{"previous_state": previous, "ticket_version": result.transferVersion})
	}
	return result, nil
}

func (s *MediaService) retryVariantUpload(ctx context.Context, existing *m.MediaAssetVariant, in CreateAssetVariantInput) (*AssetVariant, error) {
	checksum := strings.ToLower(strings.TrimSpace(in.Checksum))
	if in.Checksum != "" && (existing.ExpectedChecksum != checksum || existing.SizeBytes != in.SizeBytes || normalizeUploadMIME(existing.MimeType) != normalizeUploadMIME(in.MimeType)) {
		return nil, ErrInvalidStatusTransition
	}
	if checksum == "" || !uploadMayRetry(existing.UploadState, existing.UploadExpiresAt) {
		return toAssetVariant(existing), nil
	}
	var result *AssetVariant
	var previous string
	err := s.repo.WithVariantTransfer(ctx, strings.TrimSpace(in.TenantUUID), strings.TrimSpace(in.AssetUUID), existing.UUID.String(), func(parent *m.MediaAsset, row *m.MediaAssetVariant) error {
		if parent.UploadState == m.UploadStateDeleted {
			return ErrAssetNotFound
		}
		if row.ExpectedChecksum != checksum || row.SizeBytes != in.SizeBytes || normalizeUploadMIME(row.MimeType) == "" || normalizeUploadMIME(row.MimeType) != normalizeUploadMIME(in.MimeType) || row.TicketVersion == ^uint64(0) {
			return ErrInvalidStatusTransition
		}
		if uploadMayRetry(row.UploadState, row.UploadExpiresAt) {
			previous = row.UploadState
			expires := time.Now().Add(time.Hour).UTC()
			row.Meta = uploadRetryMetadata(row.Meta, previous, row.TicketVersion)
			row.UploadState, row.UploadExpiresAt, row.CompletedAt = m.UploadStatePending, &expires, nil
			row.TicketVersion++
		}
		result = toAssetVariant(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if previous != "" {
		s.emitAudit(ctx, in.TenantUUID, "media.variant.upload_retry", existing.UUID.String(), in.OperatorID, map[string]any{"asset_uuid": in.AssetUUID, "previous_state": previous})
	}
	return result, nil
}

func uploadRetryMetadata(raw datatypes.JSON, previous string, version uint64) datatypes.JSON {
	meta := extractMeta(raw)
	meta["upload_last_retry"] = map[string]any{"previous_state": previous, "previous_ticket_version": version, "retried_at": time.Now().UTC().Format(time.RFC3339Nano)}
	data, _ := json.Marshal(meta)
	return data
}

func uploadFailureMetadata(raw datatypes.JSON, v uploadValidation, version uint64) datatypes.JSON {
	meta := extractMeta(raw)
	failure := v.fields()
	failure["failed_at"], failure["ticket_version"] = time.Now().UTC().Format(time.RFC3339Nano), version
	history, _ := meta["upload_failures"].([]any)
	if len(history) >= 8 {
		history = history[len(history)-7:]
	}
	meta["upload_failures"] = append(history, failure)
	data, _ := json.Marshal(meta)
	return data
}
