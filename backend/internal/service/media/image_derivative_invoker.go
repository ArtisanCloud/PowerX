package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
)

const ImageDerivativesReadCapability = "com.corex.media.image_derivatives.read"
const ImageDerivativesEnsureCapability = "com.corex.media.image_derivatives.ensure"
const ImageDerivativesEndpoint = "core://media/image-derivatives"

type ImageDerivativeOperation struct {
	Operation   string `json:"operation"`
	AssetUUID   string `json:"asset_uuid"`
	RetryFailed bool   `json:"retry_failed,omitempty"`
}

type ImageDerivativeInvoker struct{ Service *MediaService }

func (i *ImageDerivativeInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID != ImageDerivativesReadCapability && in.CapabilityID != ImageDerivativesEnsureCapability {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	bad := func() (map[string]interface{}, error) {
		return nil, MediaInvalidArgumentError(errors.New("typed media derivative operation required"))
	}
	if in.Method != "INVOKE" || in.Endpoint != ImageDerivativesEndpoint || len(in.Query) > 0 || len(in.Context) > 0 {
		return bad()
	}
	for key := range in.Payload {
		if key != "body" {
			return bad()
		}
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return bad()
	}
	var op ImageDerivativeOperation
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&op) != nil || d.Decode(&struct{}{}) != io.EOF {
		return bad()
	}
	id, err := uuid.Parse(op.AssetUUID)
	if err != nil || id == uuid.Nil || id.String() != op.AssetUUID {
		return bad()
	}
	if (in.CapabilityID == ImageDerivativesReadCapability && (op.Operation != "status" || op.RetryFailed)) || (in.CapabilityID == ImageDerivativesEnsureCapability && op.Operation != "ensure") {
		return bad()
	}
	if i == nil || i.Service == nil {
		return nil, MediaUpstreamDependencyError(errors.New("media service unavailable"))
	}
	tenant := reqctx.GetTenantUUID(ctx)
	if err = i.Service.authorizeImageDerivative(ctx, in.TenantUUID, in.CapabilityID); err != nil {
		return nil, err
	}
	var result *ImageDerivatives
	if op.Operation == "ensure" {
		result, err = i.Service.EnsureImageDerivatives(ctx, tenant, op.AssetUUID, op.RetryFailed)
	} else {
		result, err = i.Service.ImageDerivativeStatus(ctx, tenant, op.AssetUUID)
	}
	if errors.Is(err, ErrAssetNotFound) {
		return nil, MediaAssetNotFoundError(err)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"result": result}, nil
}

// AuthorizeImageDerivativeRead also protects the optional service event stream.
func (s *MediaService) AuthorizeImageDerivativeRead(ctx context.Context) error {
	err := s.authorizeImageDerivative(ctx, reqctx.GetTenantUUID(ctx), ImageDerivativesReadCapability)
	var grant *cap.DirectGrantError
	if errors.As(err, &grant) {
		switch grant.Status {
		case 401:
			return MediaUnauthorizedError(err)
		case 403:
			return MediaForbiddenError(err)
		default:
			return MediaUpstreamDependencyError(err)
		}
	}
	return err
}

func (s *MediaService) authorizeImageDerivative(ctx context.Context, tenant, capabilityID string) error {
	if s == nil || s.DB == nil {
		return MediaUpstreamDependencyError(errors.New("media service unavailable"))
	}
	if tenant == "" || tenant != reqctx.GetTenantUUID(ctx) || !cap.ServiceCredential(ctx) {
		return MediaUnauthorizedError(errors.New("media service credential required"))
	}
	access, err := cap.NewGrantStatusService(s.DB).CurrentAccess(ctx)
	if err != nil {
		return err
	}
	return access.Require(capabilityID)
}
