package metadata

import (
	"context"
	"errors"

	metadatav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/metadata/v1"
	metadto "github.com/ArtisanCloud/PowerX/internal/dto/metadata"
	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ListTaxonomies(ctx context.Context, req *metadatav1.ListTaxonomiesRequest) (*metadatav1.ListTaxonomiesResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page := svcPagination(req.GetPagination())
	out, err := svc.ListTaxonomies(ctx, metasvc.ListTaxonomiesInput{
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
	items := make([]*metadatav1.Taxonomy, 0, len(out.Items))
	for i := range out.Items {
		items = append(items, protoTaxonomy(out.Items[i]))
	}
	return &metadatav1.ListTaxonomiesResponse{Data: items, Pagination: protoPagination(out.Page, out.PageSize, out.Total)}, nil
}

func (s *Server) CreateTaxonomy(ctx context.Context, req *metadatav1.CreateTaxonomyRequest) (*metadatav1.CreateTaxonomyResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.CreateTaxonomy(ctx, metasvc.CreateTaxonomyInput{
		TenantUUID:      tenantUUID,
		Namespace:       req.GetNamespace(),
		Module:          req.GetModule(),
		NameI18n:        protoI18nMap(req.GetNameI18N()),
		DescriptionI18n: protoI18nMap(req.GetDescriptionI18N()),
		MaxDepth:        int(req.GetMaxDepth()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.CreateTaxonomyResponse{Data: protoTaxonomy(out)}, nil
}

func (s *Server) UpdateTaxonomy(ctx context.Context, req *metadatav1.UpdateTaxonomyRequest) (*metadatav1.UpdateTaxonomyResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := metasvc.UpdateTaxonomyInput{TenantUUID: tenantUUID, TaxonomyUUID: req.GetTaxonomyUuid()}
	if req.GetNameI18N() != nil {
		values := protoI18nMap(req.GetNameI18N())
		in.NameI18n = &values
	}
	if req.GetDescriptionI18N() != nil {
		values := protoI18nMap(req.GetDescriptionI18N())
		in.DescriptionI18n = &values
	}
	if req.MaxDepth != nil {
		value := int(req.GetMaxDepth())
		in.MaxDepth = &value
	}
	if req.Status != nil {
		value := statusToString(req.GetStatus())
		in.Status = &value
	}
	out, err := svc.UpdateTaxonomy(ctx, in)
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.UpdateTaxonomyResponse{Data: protoTaxonomy(out)}, nil
}

func (s *Server) ListTaxonomyNodes(ctx context.Context, req *metadatav1.ListTaxonomyNodesRequest) (*metadatav1.ListTaxonomyNodesResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.ListTaxonomyNodes(ctx, metasvc.ListTaxonomyNodesInput{
		TenantUUID:   tenantUUID,
		TaxonomyUUID: req.GetTaxonomyUuid(),
		Status:       statusToString(req.GetStatus()),
		Query:        req.GetQ(),
		Locale:       req.GetLocale(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*metadatav1.TaxonomyNode, 0, len(out))
	for i := range out {
		items = append(items, protoTaxonomyNode(out[i]))
	}
	return &metadatav1.ListTaxonomyNodesResponse{Data: items}, nil
}

func (s *Server) CreateTaxonomyNode(ctx context.Context, req *metadatav1.CreateTaxonomyNodeRequest) (*metadatav1.CreateTaxonomyNodeResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	parent := optionalString(req.GetParentUuid())
	out, err := svc.CreateTaxonomyNode(ctx, metasvc.CreateTaxonomyNodeInput{
		TenantUUID:      tenantUUID,
		TaxonomyUUID:    req.GetTaxonomyUuid(),
		ParentUUID:      parent,
		Code:            req.GetCode(),
		LabelI18n:       protoI18nMap(req.GetLabelI18N()),
		DescriptionI18n: protoI18nMap(req.GetDescriptionI18N()),
		SortOrder:       int(req.GetSortOrder()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.CreateTaxonomyNodeResponse{Data: protoTaxonomyNode(out)}, nil
}

func (s *Server) UpdateTaxonomyNode(ctx context.Context, req *metadatav1.UpdateTaxonomyNodeRequest) (*metadatav1.UpdateTaxonomyNodeResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := metasvc.UpdateTaxonomyNodeInput{TenantUUID: tenantUUID, NodeUUID: req.GetNodeUuid(), Version: req.GetVersion()}
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
	out, err := svc.UpdateTaxonomyNode(ctx, in)
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.UpdateTaxonomyNodeResponse{Data: protoTaxonomyNode(out)}, nil
}

func (s *Server) MoveTaxonomyNode(ctx context.Context, req *metadatav1.MoveTaxonomyNodeRequest) (*metadatav1.MoveTaxonomyNodeResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	parent := optionalString(req.GetTargetParentUuid())
	in := metasvc.MoveTaxonomyNodeInput{TenantUUID: tenantUUID, NodeUUID: req.GetNodeUuid(), TargetParentUUID: parent, Version: req.GetVersion()}
	if req.SortOrder != nil {
		value := int(req.GetSortOrder())
		in.SortOrder = &value
	}
	out, err := svc.MoveTaxonomyNode(ctx, in)
	if err != nil {
		return nil, grpcError(err)
	}
	return &metadatav1.MoveTaxonomyNodeResponse{Data: protoTaxonomyNode(out)}, nil
}

func (s *Server) DeleteTaxonomyNode(ctx context.Context, req *metadatav1.DeleteTaxonomyNodeRequest) (*metadatav1.DeleteTaxonomyNodeResponse, error) {
	svc, err := s.metadataService()
	if err != nil {
		return nil, err
	}
	tenantUUID, err := tenantUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refs, err := svc.DeleteTaxonomyNode(ctx, metasvc.DeleteTaxonomyNodeInput{TenantUUID: tenantUUID, NodeUUID: req.GetNodeUuid()})
	if err != nil {
		if errors.Is(err, metasvc.ErrReferenceConflict) {
			out := make([]*metadatav1.ReferenceSummary, 0, len(refs))
			for i := range refs {
				out = append(out, protoReferenceSummary(refs[i]))
			}
			return &metadatav1.DeleteTaxonomyNodeResponse{Success: false, References: out}, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, grpcError(err)
	}
	return &metadatav1.DeleteTaxonomyNodeResponse{Success: true}, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func protoTaxonomy(in metadto.TaxonomyResponse) *metadatav1.Taxonomy {
	return &metadatav1.Taxonomy{
		Uuid:            in.UUID,
		Namespace:       in.Namespace,
		Module:          in.Module,
		NameI18N:        protoI18n(in.NameI18n),
		DescriptionI18N: protoI18n(in.DescriptionI18n),
		MaxDepth:        int32(in.MaxDepth),
		Status:          protoStatus(in.Status),
		Display:         protoDisplay(in.Display),
	}
}

func protoTaxonomyNode(in metadto.TaxonomyNodeResponse) *metadatav1.TaxonomyNode {
	parent := ""
	if in.ParentUUID != nil {
		parent = *in.ParentUUID
	}
	return &metadatav1.TaxonomyNode{
		Uuid:            in.UUID,
		TaxonomyUuid:    in.TaxonomyUUID,
		ParentUuid:      parent,
		Code:            in.Code,
		LabelI18N:       protoI18n(in.LabelI18n),
		DescriptionI18N: protoI18n(in.DescriptionI18n),
		Path:            in.Path,
		Depth:           int32(in.Depth),
		SortOrder:       int32(in.SortOrder),
		Status:          protoStatus(in.Status),
		ReferenceCount:  in.ReferenceCount,
		Version:         in.Version,
		Display:         protoDisplay(in.Display),
	}
}
