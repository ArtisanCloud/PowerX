package metadata

import (
	"context"
	"errors"
	"strings"

	metadatav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/metadata/v1"
	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func (s *Server) metadataService() (*metasvc.Service, error) {
	if s == nil || s.deps == nil || s.deps.DB == nil {
		return nil, status.Error(codes.FailedPrecondition, "metadata service unavailable")
	}
	svc, err := metasvc.NewService(metasvc.Deps{DB: s.deps.DB, ValidatorRegistry: s.deps.MetadataResourceValidatorRegistry})
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return svc, nil
}

func tenantUUIDFromContext(ctx context.Context) (string, error) {
	raw := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if raw == "" {
		return "", status.Error(codes.InvalidArgument, "tenant uuid is required")
	}
	canonical, err := reqctx.CanonicalTenantUUID(raw)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "tenant uuid is invalid")
	}
	return canonical, nil
}

func (s *Server) ListDictionaryNamespaces(ctx context.Context, req *metadatav1.ListDictionaryNamespacesRequest) (*metadatav1.ListDictionaryNamespacesResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page := svcPagination(req.GetPagination())
	out, err := svc.ListDictionaryNamespaces(ctx, metasvc.ListDictionaryNamespacesInput{
		TenantUUID: tenantUUID,
		Module:     req.GetModule(),
		Status:     statusToString(req.GetStatus()),
		Query:      req.GetQ(),
		Locale:     req.GetLocale(),
		Page:       page.page,
		PageSize:   page.pageSize,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*metadatav1.DictionaryNamespace, 0, len(out.Items))
	for i := range out.Items {
		items = append(items, protoDictionaryNamespace(out.Items[i]))
	}
	return &metadatav1.ListDictionaryNamespacesResponse{
		Data:       items,
		Pagination: protoPagination(out.Page, out.PageSize, out.Total),
	}, nil
}

func (s *Server) CreateDictionaryNamespace(ctx context.Context, req *metadatav1.CreateDictionaryNamespaceRequest) (*metadatav1.CreateDictionaryNamespaceResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.CreateDictionaryNamespace(ctx, metasvc.CreateDictionaryNamespaceInput{
		TenantUUID:      tenantUUID,
		Namespace:       req.GetNamespace(),
		Module:          req.GetModule(),
		NameI18n:        protoI18nMap(req.GetNameI18N()),
		DescriptionI18n: protoI18nMap(req.GetDescriptionI18N()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.CreateDictionaryNamespaceResponse{Data: protoDictionaryNamespace(out)}, nil
}

func (s *Server) UpdateDictionaryNamespace(ctx context.Context, req *metadatav1.UpdateDictionaryNamespaceRequest) (*metadatav1.UpdateDictionaryNamespaceResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := metasvc.UpdateDictionaryNamespaceInput{
		TenantUUID:    tenantUUID,
		NamespaceUUID: req.GetNamespaceUuid(),
	}
	if req.GetNameI18N() != nil {
		values := protoI18nMap(req.GetNameI18N())
		in.NameI18n = &values
	}
	if req.GetDescriptionI18N() != nil {
		values := protoI18nMap(req.GetDescriptionI18N())
		in.DescriptionI18n = &values
	}
	if req.Status != nil {
		value := statusToString(req.GetStatus())
		in.Status = &value
	}
	out, err := svc.UpdateDictionaryNamespace(ctx, in)
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.UpdateDictionaryNamespaceResponse{Data: protoDictionaryNamespace(out)}, nil
}

func (s *Server) ListDictionaryItems(ctx context.Context, req *metadatav1.ListDictionaryItemsRequest) (*metadatav1.ListDictionaryItemsResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page := svcPagination(req.GetPagination())
	out, err := svc.ListDictionaryItems(ctx, metasvc.ListDictionaryItemsInput{
		TenantUUID:    tenantUUID,
		NamespaceUUID: req.GetNamespaceUuid(),
		Status:        statusToString(req.GetStatus()),
		Query:         req.GetQ(),
		Locale:        req.GetLocale(),
		Page:          page.page,
		PageSize:      page.pageSize,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*metadatav1.DictionaryItem, 0, len(out.Items))
	for i := range out.Items {
		items = append(items, protoDictionaryItem(out.Items[i]))
	}
	return &metadatav1.ListDictionaryItemsResponse{
		Data:       items,
		Pagination: protoPagination(out.Page, out.PageSize, out.Total),
	}, nil
}

func (s *Server) CreateDictionaryItem(ctx context.Context, req *metadatav1.CreateDictionaryItemRequest) (*metadatav1.CreateDictionaryItemResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.CreateDictionaryItem(ctx, metasvc.CreateDictionaryItemInput{
		TenantUUID:      tenantUUID,
		NamespaceUUID:   req.GetNamespaceUuid(),
		Code:            req.GetCode(),
		LabelI18n:       protoI18nMap(req.GetLabelI18N()),
		DescriptionI18n: protoI18nMap(req.GetDescriptionI18N()),
		SortOrder:       int(req.GetSortOrder()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.CreateDictionaryItemResponse{Data: protoDictionaryItem(out)}, nil
}

func (s *Server) UpdateDictionaryItem(ctx context.Context, req *metadatav1.UpdateDictionaryItemRequest) (*metadatav1.UpdateDictionaryItemResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := metasvc.UpdateDictionaryItemInput{
		TenantUUID: tenantUUID,
		ItemUUID:   req.GetItemUuid(),
	}
	if req.GetLabelI18N() != nil {
		values := protoI18nMap(req.GetLabelI18N())
		in.LabelI18n = &values
	}
	if req.GetDescriptionI18N() != nil {
		values := protoI18nMap(req.GetDescriptionI18N())
		in.DescriptionI18n = &values
	}
	if req.SortOrder != nil {
		value := int(req.GetSortOrder())
		in.SortOrder = &value
	}
	if req.Status != nil {
		value := statusToString(req.GetStatus())
		in.Status = &value
	}
	out, err := svc.UpdateDictionaryItem(ctx, in)
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.UpdateDictionaryItemResponse{Data: protoDictionaryItem(out)}, nil
}

func (s *Server) DeleteDictionaryItem(ctx context.Context, req *metadatav1.DeleteDictionaryItemRequest) (*metadatav1.DeleteDictionaryItemResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refs, err := svc.DeleteDictionaryItem(ctx, metasvc.DeleteDictionaryItemInput{
		TenantUUID: tenantUUID,
		ItemUUID:   req.GetItemUuid(),
	})
	if err != nil {
		if errors.Is(err, metasvc.ErrReferenceConflict) {
			out := make([]*metadatav1.ReferenceSummary, 0, len(refs))
			for i := range refs {
				out = append(out, protoReferenceSummary(refs[i]))
			}
			return &metadatav1.DeleteDictionaryItemResponse{Success: false, References: out}, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, grpcError(err)
	}
	return &metadatav1.DeleteDictionaryItemResponse{Success: true}, nil
}

type paginationInput struct {
	page     int
	pageSize int
}

func svcPagination(in *metadatav1.PaginationRequest) paginationInput {
	if in == nil {
		return paginationInput{}
	}
	return paginationInput{page: int(in.GetPage()), pageSize: int(in.GetPageSize())}
}

func protoI18nMap(in *metadatav1.I18NMap) map[string]string {
	if in == nil || len(in.GetValues()) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in.GetValues()))
	for k, v := range in.GetValues() {
		out[k] = v
	}
	return out
}

func protoI18n(values metadto.I18nMap) *metadatav1.I18NMap {
	out := &metadatav1.I18NMap{Values: map[string]string{}}
	for k, v := range values {
		out.Values[k] = v
	}
	return out
}

func protoDisplay(in metadto.Display) *metadatav1.Display {
	return &metadatav1.Display{
		DisplayName:          in.DisplayName,
		DisplayDescription:   in.DisplayDescription,
		DisplayLocale:        in.DisplayLocale,
		DisplayLocaleMissing: in.DisplayLocaleMissing,
	}
}

func protoDictionaryNamespace(in metadto.DictionaryNamespaceResponse) *metadatav1.DictionaryNamespace {
	return &metadatav1.DictionaryNamespace{
		Uuid:            in.UUID,
		Namespace:       in.Namespace,
		Module:          in.Module,
		NameI18N:        protoI18n(in.NameI18n),
		DescriptionI18N: protoI18n(in.DescriptionI18n),
		Status:          protoStatus(in.Status),
		ItemCount:       in.ItemCount,
		Display:         protoDisplay(in.Display),
	}
}

func protoDictionaryItem(in metadto.DictionaryItemResponse) *metadatav1.DictionaryItem {
	return &metadatav1.DictionaryItem{
		Uuid:            in.UUID,
		NamespaceUuid:   in.NamespaceUUID,
		Code:            in.Code,
		LabelI18N:       protoI18n(in.LabelI18n),
		DescriptionI18N: protoI18n(in.DescriptionI18n),
		SortOrder:       int32(in.SortOrder),
		Status:          protoStatus(in.Status),
		ReferenceCount:  in.ReferenceCount,
		Display:         protoDisplay(in.Display),
	}
}

func protoPagination(page int, pageSize int, total int64) *metadatav1.PaginationResponse {
	return &metadatav1.PaginationResponse{
		Page:     int32(page),
		PageSize: int32(pageSize),
		Total:    total,
	}
}

func protoReferenceSummary(in metadto.ReferenceSummary) *metadatav1.ReferenceSummary {
	return &metadatav1.ReferenceSummary{
		ResourceType: in.ResourceType,
		ResourceUuid: in.ResourceUUID,
		FieldName:    in.FieldName,
		Count:        in.Count,
	}
}

func protoStatus(value string) metadatav1.MetadataStatus {
	switch strings.TrimSpace(value) {
	case model.StatusEnabled:
		return metadatav1.MetadataStatus_METADATA_STATUS_ENABLED
	case model.StatusDisabled:
		return metadatav1.MetadataStatus_METADATA_STATUS_DISABLED
	case model.StatusArchived:
		return metadatav1.MetadataStatus_METADATA_STATUS_ARCHIVED
	default:
		return metadatav1.MetadataStatus_METADATA_STATUS_UNSPECIFIED
	}
}

func statusToString(value metadatav1.MetadataStatus) string {
	switch value {
	case metadatav1.MetadataStatus_METADATA_STATUS_ENABLED:
		return model.StatusEnabled
	case metadatav1.MetadataStatus_METADATA_STATUS_DISABLED:
		return model.StatusDisabled
	case metadatav1.MetadataStatus_METADATA_STATUS_ARCHIVED:
		return model.StatusArchived
	default:
		return ""
	}
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, metasvc.ErrInvalidMachineIdentifier),
		errors.Is(err, metasvc.ErrMissingRequiredLocale),
		errors.Is(err, metasvc.ErrInvalidStatus),
		errors.Is(err, metasvc.ErrInvalidDepth),
		errors.Is(err, metasvc.ErrUUIDRequired),
		errors.Is(err, metasvc.ErrMergeSameTag):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, metasvc.ErrReferenceConflict),
		errors.Is(err, metasvc.ErrCircularMove),
		errors.Is(err, metasvc.ErrOptimisticConflict),
		errors.Is(err, metasvc.ErrHasChildNodes),
		errors.Is(err, metasvc.ErrTagBound),
		errors.Is(err, metasvc.ErrTagResourceMismatch),
		errors.Is(err, metasvc.ErrReferenceResourceMismatch),
		errors.Is(err, metasvc.ErrTagDisabled),
		errors.Is(err, metasvc.ErrResourceBindingDisabled),
		errors.Is(err, metasvc.ErrResourceValidatorMissing):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, metasvc.ErrResourceTypeMissing):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
