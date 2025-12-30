package media

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	corexmediav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/media/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MediaAssetServer 提供 gRPC 媒体资产管理服务。
type MediaAssetServer struct {
	corexmediav1.UnimplementedMediaAssetAdminServiceServer
	svc *mediasvc.MediaService
}

// NewMediaAssetServer 构建服务实例。
func NewMediaAssetServer(deps *shared.Deps) *MediaAssetServer {
	return &MediaAssetServer{svc: deps.MediaSvc}
}

func (s *MediaAssetServer) CreateMediaAsset(ctx context.Context, req *corexmediav1.CreateMediaAssetRequest) (*corexmediav1.MediaAssetResponse, error) {
	tenantUUID, err := resolveTenantUUID(ctx, req.GetTenantUuid())
	if err != nil {
		return mediaErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
	}
	operator := parseOperatorID(req.GetOperatorId())

	asset, err := s.svc.CreateAsset(ctx, mediasvc.CreateAssetInput{
		TenantUUID:   tenantUUID,
		OperatorID:   operator,
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		Driver:       req.GetDriver(),
		Folder:       req.GetFolder(),
		OwnerType:    req.GetOwnerSubjectType(),
		OwnerID:      req.GetOwnerSubjectId(),
		Tags:         req.GetTags(),
		UploadMethod: uploadMethodFromChannel(req.GetUploadChannel()),
		ExternalURL:  req.GetExternalUrl(),
	})
	if err != nil {
		return mediaServiceErrorResponse(ctx, err)
	}
	return &corexmediav1.MediaAssetResponse{
		Meta: okMeta(ctx),
		Data: toPBAsset(asset),
	}, nil
}

func (s *MediaAssetServer) ListMediaAssets(ctx context.Context, req *corexmediav1.ListMediaAssetsRequest) (*corexmediav1.ListMediaAssetsResponse, error) {
	tenantUUID, err := resolveTenantUUID(ctx, req.GetTenantUuid())
	if err != nil {
		return listMediaErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
	}
	filter := mediasvc.ListAssetsInput{
		TenantUUID: tenantUUID,
		Drivers:    optionalSlice(req.GetDriver()),
		OwnerType:  req.GetOwnerSubjectType(),
		OwnerID:    req.GetOwnerSubjectId(),
		Keyword:    req.GetKeyword(),
		TagsAll:    req.GetTags(),
		Page:       int(req.GetPage()),
		PageSize:   int(req.GetPageSize()),
	}
	if statusEnum := req.GetBusinessStatus(); statusEnum != corexmediav1.BusinessStatus_BUSINESS_STATUS_UNSPECIFIED {
		filter.BusinessStatus = []string{statusToString(statusEnum)}
	}
	assets, total, err := s.svc.ListAssets(ctx, filter)
	if err != nil {
		return listMediaServiceErrorResponse(ctx, err)
	}
	items := make([]*corexmediav1.MediaAsset, 0, len(assets))
	for i := range assets {
		items = append(items, toPBAsset(&assets[i]))
	}
	pageSize := req.GetPageSize()
	if pageSize <= 0 {
		pageSize = 20
	}
	page := req.GetPage()
	if page <= 0 {
		page = 1
	}
	pageResp := &commonv1.PageResponse{
		Total: int64(total),
	}
	if int64(page)*int64(pageSize) < total {
		pageResp.NextPageToken = strconv.FormatInt(int64(page)+1, 10)
	}
	return &corexmediav1.ListMediaAssetsResponse{
		Meta: okMeta(ctx),
		Data: &corexmediav1.ListMediaAssetsData{
			Items: items,
			Page:  pageResp,
		},
	}, nil
}

func (s *MediaAssetServer) GetMediaAsset(ctx context.Context, req *corexmediav1.GetMediaAssetRequest) (*corexmediav1.MediaAssetResponse, error) {
	tenantUUID, err := resolveTenantUUID(ctx, req.GetTenantUuid())
	if err != nil {
		return mediaErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
	}
	asset, err := s.svc.GetAsset(ctx, tenantUUID, req.GetUuid(), false)
	if err != nil {
		return mediaServiceErrorResponse(ctx, err)
	}
	return &corexmediav1.MediaAssetResponse{
		Meta: okMeta(ctx),
		Data: toPBAsset(asset),
	}, nil
}

func (s *MediaAssetServer) UpdateMediaAsset(ctx context.Context, req *corexmediav1.UpdateMediaAssetRequest) (*corexmediav1.MediaAssetResponse, error) {
	tenantUUID, err := resolveTenantUUID(ctx, req.GetTenantUuid())
	if err != nil {
		return mediaErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
	}
	operator := parseOperatorID(req.GetOperatorId())
	input := mediasvc.UpdateAssetInput{
		TenantUUID: tenantUUID,
		UUID:       req.GetUuid(),
		OperatorID: operator,
		Tags:       req.GetTags(),
	}
	if req.Name != nil {
		input.Name = req.Name
	}
	if req.Description != nil {
		input.Description = req.Description
	}
	if req.BusinessStatus != nil {
		statusString := statusToString(req.GetBusinessStatus())
		input.BusinessStatus = &statusString
	}
	asset, err := s.svc.UpdateAsset(ctx, input)
	if err != nil {
		return mediaServiceErrorResponse(ctx, err)
	}
	return &corexmediav1.MediaAssetResponse{
		Meta: okMeta(ctx),
		Data: toPBAsset(asset),
	}, nil
}

func (s *MediaAssetServer) DeleteMediaAsset(ctx context.Context, req *corexmediav1.DeleteMediaAssetRequest) (*corexmediav1.DeleteMediaAssetResponse, error) {
	tenantUUID, err := resolveTenantUUID(ctx, req.GetTenantUuid())
	if err != nil {
		return deleteMediaErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
	}
	operator := parseOperatorID(req.GetOperatorId())
	err = s.svc.DeleteAsset(ctx, mediasvc.DeleteAssetInput{
		TenantUUID: tenantUUID,
		UUID:       req.GetUuid(),
		OperatorID: operator,
	})
	if err != nil {
		return deleteMediaServiceErrorResponse(ctx, err)
	}
	return &corexmediav1.DeleteMediaAssetResponse{
		Meta: okMeta(ctx),
		Data: &corexmediav1.DeleteMediaAssetData{
			Deleted: true,
		},
	}, nil
}

func (s *MediaAssetServer) PresignMediaAsset(ctx context.Context, req *corexmediav1.PresignMediaAssetRequest) (*corexmediav1.PresignMediaAssetResponse, error) {
	tenantUUID, err := resolveTenantUUID(ctx, req.GetTenantUuid())
	if err != nil {
		return presignMediaErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
	}
	operator := parseOperatorID(req.GetOperatorId())
	ttl := time.Duration(req.GetExpiresInSeconds()) * time.Second
	headers := http.Header{}
	for k, v := range req.GetMetadata() {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		headers.Set(k, v)
	}

	out, err := s.svc.PresignAsset(ctx, mediasvc.PresignAssetInput{
		TenantUUID: tenantUUID,
		UUID:       req.GetUuid(),
		OperatorID: operator,
		Action:     presignActionToString(req.GetAction()),
		Method:     req.GetMethod(),
		TTL:        ttl,
		Headers:    headers,
	})
	if err != nil {
		return presignMediaServiceErrorResponse(ctx, err)
	}
	expiresIn := time.Until(out.ExpireAt)
	if expiresIn < 0 {
		expiresIn = 0
	}
	return &corexmediav1.PresignMediaAssetResponse{
		Meta: okMeta(ctx),
		Data: &corexmediav1.PresignMediaAssetData{
			Url:              out.URL,
			Method:           out.Method,
			ExpiresInSeconds: uint32(expiresIn / time.Second),
			Headers:          headersToMap(out.Headers),
			ObjectKey:        out.ObjectKey,
		},
	}, nil
}

func toPBAsset(asset *mediasvc.Asset) *corexmediav1.MediaAsset {
	if asset == nil {
		return nil
	}
	pb := &corexmediav1.MediaAsset{
		Uuid:             asset.UUID,
		TenantUuid:       asset.TenantUUID,
		Name:             asset.Name,
		Description:      asset.Description,
		Driver:           asset.Driver,
		Folder:           asset.Folder,
		ObjectKey:        asset.StorageKey,
		ExternalUrl:      asset.ExternalURL,
		SizeBytes:        asset.SizeBytes,
		MimeType:         asset.MimeType,
		OwnerSubjectType: asset.OwnerType,
		OwnerSubjectId:   asset.OwnerID,
		Tags:             append([]string(nil), asset.Tags...),
		BusinessStatus:   stringToStatus(asset.BusinessStatus),
		DownloadUrl:      asset.DownloadURL,
		Extra:            metadataToStringMap(asset.Metadata),
		Audit: &corexmediav1.AuditMetadata{
			CreatedBy: pointerToString(asset.CreatedBy),
			UpdatedBy: pointerToString(asset.UpdatedBy),
			CreatedAt: asset.CreatedAt.Unix(),
			UpdatedAt: asset.UpdatedAt.Unix(),
		},
	}
	if asset.DownloadExpiry != nil {
		pb.DownloadExpiredAt = asset.DownloadExpiry.Unix()
	}
	return pb
}

func parseOperatorID(raw string) *uint64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if id, err := strconv.ParseUint(raw, 10, 64); err == nil {
		return &id
	}
	return nil
}

func pointerToString(v *uint64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatUint(*v, 10)
}

func uploadMethodFromChannel(ch corexmediav1.UploadChannel) mediasvc.UploadMethod {
	switch ch {
	case corexmediav1.UploadChannel_UPLOAD_CHANNEL_EXTERNAL_URL:
		return mediasvc.UploadMethodExternalLink
	case corexmediav1.UploadChannel_UPLOAD_CHANNEL_PRESIGNED:
		return mediasvc.UploadMethodPresign
	default:
		return mediasvc.UploadMethodDirect
	}
}

func statusToString(st corexmediav1.BusinessStatus) string {
	switch st {
	case corexmediav1.BusinessStatus_BUSINESS_STATUS_UNDER_REVIEW:
		return "under_review"
	case corexmediav1.BusinessStatus_BUSINESS_STATUS_PUBLISHED:
		return "published"
	case corexmediav1.BusinessStatus_BUSINESS_STATUS_ARCHIVED:
		return "archived"
	default:
		return "draft"
	}
}

func stringToStatus(status string) corexmediav1.BusinessStatus {
	switch status {
	case "under_review":
		return corexmediav1.BusinessStatus_BUSINESS_STATUS_UNDER_REVIEW
	case "published":
		return corexmediav1.BusinessStatus_BUSINESS_STATUS_PUBLISHED
	case "archived":
		return corexmediav1.BusinessStatus_BUSINESS_STATUS_ARCHIVED
	default:
		return corexmediav1.BusinessStatus_BUSINESS_STATUS_DRAFT
	}
}

func optionalSlice(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{strings.TrimSpace(value)}
}

func resolveTenantUUID(ctx context.Context, payload string) (string, error) {
	if uuidStr := strings.TrimSpace(reqctx.GetTenantUUID(ctx)); uuidStr != "" {
		canonical, err := reqctx.CanonicalTenantUUID(uuidStr)
		if err != nil {
			return "", status.Error(codes.InvalidArgument, "tenant uuid is invalid")
		}
		return canonical, nil
	}
	if trimmed := strings.TrimSpace(payload); trimmed != "" {
		canonical, err := reqctx.CanonicalTenantUUID(trimmed)
		if err != nil {
			return "", status.Error(codes.InvalidArgument, "tenant uuid is invalid")
		}
		return canonical, nil
	}
	if _, err := reqctx.RequireTenantUUID(ctx); err != nil {
		return "", err
	}
	return "", errors.New("tenant uuid required")
}

func headersToMap(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(headers))
	for k, values := range headers {
		if len(values) > 0 {
			result[k] = values[0]
		}
	}
	return result
}

func metadataToStringMap(meta map[string]any) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		switch val := v.(type) {
		case string:
			out[k] = val
		case fmt.Stringer:
			out[k] = val.String()
		default:
			out[k] = fmt.Sprint(val)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func presignActionToString(action corexmediav1.PresignAction) string {
	switch action {
	case corexmediav1.PresignAction_PRESIGN_ACTION_UPLOAD:
		return "upload"
	case corexmediav1.PresignAction_PRESIGN_ACTION_DOWNLOAD:
		return "download"
	default:
		return "download"
	}
}

func okMeta(ctx context.Context) *commonv1.ResponseMeta {
	reqID := reqctx.GetTraceID(ctx)
	return &commonv1.ResponseMeta{
		Code:      http.StatusOK,
		Message:   "success",
		Timestamp: time.Now().Unix(),
		RequestId: reqID,
	}
}

func badMeta(ctx context.Context, code int, msg string) *commonv1.ResponseMeta {
	reqID := reqctx.GetTraceID(ctx)
	return &commonv1.ResponseMeta{
		Code:      int32(code),
		Message:   strings.TrimSpace(msg),
		Timestamp: time.Now().Unix(),
		RequestId: reqID,
	}
}

func errorExtra(err error) *commonv1.ErrorExtra {
	if err == nil {
		return nil
	}
	return &commonv1.ErrorExtra{Error: err.Error()}
}

func classifyMediaError(err error) (int, bool) {
	switch {
	case err == nil:
		return http.StatusOK, false
	case errors.Is(err, mediasvc.ErrAssetNotFound):
		return http.StatusNotFound, false
	case errors.Is(err, mediasvc.ErrInvalidStatusTransition):
		return http.StatusPreconditionFailed, false
	case errors.Is(err, mediasvc.ErrInvalidUploadMethod), errors.Is(err, mediasvc.ErrExternalURLRequired):
		return http.StatusBadRequest, false
	case errors.Is(err, driver.ErrInvalidArgument):
		return http.StatusBadRequest, false
	case errors.Is(err, driver.ErrPermission):
		return http.StatusForbidden, false
	case errors.Is(err, driver.ErrConflict):
		return http.StatusConflict, false
	case errors.Is(err, mediamgr.ErrDriverNotFound), errors.Is(err, mediamgr.ErrNoDefaultDriver):
		return http.StatusBadRequest, false
	default:
		return http.StatusInternalServerError, true
	}
}

func mediaServiceErrorResponse(ctx context.Context, err error) (*corexmediav1.MediaAssetResponse, error) {
	code, internal := classifyMediaError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "media service error: %v", err)
	}
	return &corexmediav1.MediaAssetResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err),
	}, nil
}

func listMediaServiceErrorResponse(ctx context.Context, err error) (*corexmediav1.ListMediaAssetsResponse, error) {
	code, internal := classifyMediaError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "media service error: %v", err)
	}
	return &corexmediav1.ListMediaAssetsResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err),
	}, nil
}

func deleteMediaServiceErrorResponse(ctx context.Context, err error) (*corexmediav1.DeleteMediaAssetResponse, error) {
	code, internal := classifyMediaError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "media service error: %v", err)
	}
	return &corexmediav1.DeleteMediaAssetResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err),
	}, nil
}

func presignMediaServiceErrorResponse(ctx context.Context, err error) (*corexmediav1.PresignMediaAssetResponse, error) {
	code, internal := classifyMediaError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "media service error: %v", err)
	}
	return &corexmediav1.PresignMediaAssetResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err),
	}, nil
}

func mediaErrorResponse(ctx context.Context, code int, msg string, err error) *corexmediav1.MediaAssetResponse {
	return &corexmediav1.MediaAssetResponse{
		Meta:  badMeta(ctx, code, msg),
		Error: errorExtra(err),
	}
}

func listMediaErrorResponse(ctx context.Context, code int, msg string, err error) *corexmediav1.ListMediaAssetsResponse {
	return &corexmediav1.ListMediaAssetsResponse{
		Meta:  badMeta(ctx, code, msg),
		Error: errorExtra(err),
	}
}

func deleteMediaErrorResponse(ctx context.Context, code int, msg string, err error) *corexmediav1.DeleteMediaAssetResponse {
	return &corexmediav1.DeleteMediaAssetResponse{
		Meta:  badMeta(ctx, code, msg),
		Error: errorExtra(err),
	}
}

func presignMediaErrorResponse(ctx context.Context, code int, msg string, err error) *corexmediav1.PresignMediaAssetResponse {
	return &corexmediav1.PresignMediaAssetResponse{
		Meta:  badMeta(ctx, code, msg),
		Error: errorExtra(err),
	}
}
