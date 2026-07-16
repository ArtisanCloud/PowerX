package metadata

import (
	"context"

	metadatav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/metadata/v1"
	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
)

func (s *Server) ListTags(ctx context.Context, req *metadatav1.ListTagsRequest) (*metadatav1.ListTagsResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page := svcPagination(req.GetPagination())
	out, err := svc.ListTags(ctx, metasvc.ListTagsInput{
		TenantUUID:   tenantUUID,
		Namespace:    req.GetNamespace(),
		ResourceType: req.GetResourceType(),
		Status:       statusToString(req.GetStatus()),
		Query:        req.GetQ(),
		Locale:       req.GetLocale(),
		Page:         page.page,
		PageSize:     page.pageSize,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*metadatav1.Tag, 0, len(out.Items))
	for i := range out.Items {
		items = append(items, protoTag(out.Items[i]))
	}
	return &metadatav1.ListTagsResponse{Data: items, Pagination: protoPagination(out.Page, out.PageSize, out.Total)}, nil
}

func (s *Server) CreateTag(ctx context.Context, req *metadatav1.CreateTagRequest) (*metadatav1.CreateTagResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.CreateTag(ctx, metasvc.CreateTagInput{
		TenantUUID:      tenantUUID,
		Namespace:       req.GetNamespace(),
		ResourceType:    req.GetResourceType(),
		Code:            req.GetCode(),
		LabelI18n:       protoI18nMap(req.GetLabelI18N()),
		DescriptionI18n: protoI18nMap(req.GetDescriptionI18N()),
		Color:           req.GetColor(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.CreateTagResponse{Data: protoTag(out)}, nil
}

func (s *Server) UpdateTag(ctx context.Context, req *metadatav1.UpdateTagRequest) (*metadatav1.UpdateTagResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := metasvc.UpdateTagInput{TenantUUID: tenantUUID, TagUUID: req.GetTagUuid()}
	if req.GetLabelI18N() != nil {
		values := protoI18nMap(req.GetLabelI18N())
		in.LabelI18n = &values
	}
	if req.GetDescriptionI18N() != nil {
		values := protoI18nMap(req.GetDescriptionI18N())
		in.DescriptionI18n = &values
	}
	if req.Color != nil {
		value := req.GetColor()
		in.Color = &value
	}
	if req.Status != nil {
		value := statusToString(req.GetStatus())
		in.Status = &value
	}
	out, err := svc.UpdateTag(ctx, in)
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.UpdateTagResponse{Data: protoTag(out)}, nil
}

func (s *Server) MergeTags(ctx context.Context, req *metadatav1.MergeTagsRequest) (*metadatav1.MergeTagsResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	moved, err := svc.MergeTags(ctx, metasvc.MergeTagsInput{
		TenantUUID:    tenantUUID,
		SourceTagUUID: req.GetSourceTagUuid(),
		TargetTagUUID: req.GetTargetTagUuid(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.MergeTagsResponse{MovedBindings: moved}, nil
}

func (s *Server) DeleteTag(ctx context.Context, req *metadatav1.DeleteTagRequest) (*metadatav1.DeleteTagResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := svc.DeleteTag(ctx, metasvc.DeleteTagInput{TenantUUID: tenantUUID, TagUUID: req.GetTagUuid()}); err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.DeleteTagResponse{Success: true}, nil
}

func (s *Server) ListTagBindings(ctx context.Context, req *metadatav1.ListTagBindingsRequest) (*metadatav1.ListTagBindingsResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.ListTagBindings(ctx, metasvc.ListTagBindingsInput{
		TenantUUID:   tenantUUID,
		ResourceType: req.GetResourceType(),
		ResourceUUID: req.GetResourceUuid(),
		Locale:       req.GetLocale(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*metadatav1.TagBinding, 0, len(out))
	for i := range out {
		items = append(items, protoTagBinding(out[i]))
	}
	return &metadatav1.ListTagBindingsResponse{Data: items}, nil
}

func (s *Server) ReplaceTagBindings(ctx context.Context, req *metadatav1.ReplaceTagBindingsRequest) (*metadatav1.ReplaceTagBindingsResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.ReplaceTagBindings(ctx, metasvc.ReplaceTagBindingsInput{
		TenantUUID:   tenantUUID,
		ResourceType: req.GetResourceType(),
		ResourceUUID: req.GetResourceUuid(),
		TagUUIDs:     req.GetTagUuids(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*metadatav1.TagBinding, 0, len(out))
	for i := range out {
		items = append(items, protoTagBinding(out[i]))
	}
	return &metadatav1.ReplaceTagBindingsResponse{Data: items}, nil
}

func protoTag(in metadto.TagResponse) *metadatav1.Tag {
	return &metadatav1.Tag{
		Uuid:            in.UUID,
		Namespace:       in.Namespace,
		ResourceType:    in.ResourceType,
		Code:            in.Code,
		LabelI18N:       protoI18n(in.LabelI18n),
		DescriptionI18N: protoI18n(in.DescriptionI18n),
		Color:           in.Color,
		Status:          protoStatus(in.Status),
		UsageCount:      in.UsageCount,
		Display:         protoDisplay(in.Display),
	}
}

func protoTagBinding(in metadto.TagBindingResponse) *metadatav1.TagBinding {
	var tag *metadatav1.Tag
	if in.Tag != nil {
		tag = protoTag(*in.Tag)
	}
	return &metadatav1.TagBinding{
		TagUuid:      in.TagUUID,
		ResourceType: in.ResourceType,
		ResourceUuid: in.ResourceUUID,
		Tag:          tag,
	}
}
