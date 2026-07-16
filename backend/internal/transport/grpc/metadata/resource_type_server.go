package metadata

import (
	"context"

	metadatav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/metadata/v1"
	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
)

func (s *Server) ListResourceTypes(ctx context.Context, req *metadatav1.ListResourceTypesRequest) (*metadatav1.ListResourceTypesResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page := svcPagination(req.GetPagination())
	out, err := svc.ListResourceTypes(ctx, metasvc.ListResourceTypesInput{
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
	items := make([]*metadatav1.ResourceType, 0, len(out.Items))
	for i := range out.Items {
		items = append(items, protoResourceType(out.Items[i]))
	}
	return &metadatav1.ListResourceTypesResponse{Data: items, Pagination: protoPagination(out.Page, out.PageSize, out.Total)}, nil
}

func (s *Server) RegisterResourceType(ctx context.Context, req *metadatav1.RegisterResourceTypeRequest) (*metadatav1.RegisterResourceTypeResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.RegisterResourceType(ctx, metasvc.RegisterResourceTypeInput{
		TenantUUID:      tenantUUID,
		ResourceType:    req.GetResourceType(),
		Module:          req.GetModule(),
		NameI18n:        protoI18nMap(req.GetNameI18N()),
		DescriptionI18n: protoI18nMap(req.GetDescriptionI18N()),
		ValidatorKey:    req.GetValidatorKey(),
		BindingEnabled:  req.GetBindingEnabled(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.RegisterResourceTypeResponse{Data: protoResourceType(out)}, nil
}

func (s *Server) UpdateResourceType(ctx context.Context, req *metadatav1.UpdateResourceTypeRequest) (*metadatav1.UpdateResourceTypeResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := metasvc.UpdateResourceTypeInput{
		TenantUUID:       tenantUUID,
		ResourceTypeUUID: req.GetResourceTypeUuid(),
	}
	if req.GetNameI18N() != nil {
		values := protoI18nMap(req.GetNameI18N())
		in.NameI18n = &values
	}
	if req.GetDescriptionI18N() != nil {
		values := protoI18nMap(req.GetDescriptionI18N())
		in.DescriptionI18n = &values
	}
	if req.ValidatorKey != nil {
		value := req.GetValidatorKey()
		in.ValidatorKey = &value
	}
	if req.BindingEnabled != nil {
		value := req.GetBindingEnabled()
		in.BindingEnabled = &value
	}
	if req.Status != nil {
		value := statusToString(req.GetStatus())
		in.Status = &value
	}
	out, err := svc.UpdateResourceType(ctx, in)
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.UpdateResourceTypeResponse{Data: protoResourceType(out)}, nil
}

func protoResourceType(in metadto.ResourceTypeResponse) *metadatav1.ResourceType {
	return &metadatav1.ResourceType{
		Uuid:            in.UUID,
		ResourceType:    in.ResourceType,
		Module:          in.Module,
		NameI18N:        protoI18n(in.NameI18n),
		DescriptionI18N: protoI18n(in.DescriptionI18n),
		ValidatorKey:    in.ValidatorKey,
		BindingEnabled:  in.BindingEnabled,
		ValidatorStatus: in.ValidatorStatus,
		Status:          protoStatus(in.Status),
		Display:         protoDisplay(in.Display),
	}
}
