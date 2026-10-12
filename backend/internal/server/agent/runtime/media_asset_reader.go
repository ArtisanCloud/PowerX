package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
)

type MediaAssetReader struct{ media *mediasvc.MediaService }

func NewMediaAssetReader(media *mediasvc.MediaService) MediaAssetReader {
	return MediaAssetReader{media: media}
}

func (r MediaAssetReader) ReadObservation(ctx context.Context, resource ResourceDescriptor, purpose string) (ResourceObservation, error) {
	if r.media == nil || resource.Kind != ResourceKindMediaAsset || !resource.ReadGranted || strings.TrimSpace(purpose) == "" {
		return ResourceObservation{}, fmt.Errorf("media asset observation is invalid")
	}
	asset, err := r.media.GetAsset(ctx, resource.TenantUUID, resource.ResourceUUID.String(), false)
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("load media asset observation: %w", err)
	}
	summary, err := json.Marshal(map[string]any{"resource_uuid": resource.ResourceUUID.String(), "kind": ResourceKindMediaAsset, "name": asset.Name, "mime_type": asset.MimeType, "size_bytes": asset.SizeBytes, "upload_state": asset.UploadState, "business_status": asset.BusinessStatus, "purpose": purpose})
	if err != nil {
		return ResourceObservation{}, err
	}
	return ResourceObservation{ResourceUUID: resource.ResourceUUID, Summary: string(summary), ArtifactRef: "media_asset/" + resource.ResourceUUID.String()}, nil
}
