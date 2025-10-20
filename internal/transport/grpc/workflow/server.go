package workflow

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
)

// Server 提供 WorkflowService 的 gRPC 桥接。
type Server struct {
	workflowv1.UnimplementedWorkflowServiceServer

	svc *workflowsvc.Service
}

// NewServer 构建 gRPC server 实例。
func NewServer(svc *workflowsvc.Service) *Server {
	return &Server{svc: svc}
}

func (s *Server) CreateDefinition(ctx context.Context, req *workflowv1.CreateDefinitionRequest) (*workflowv1.CreateDefinitionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateDefinition not implemented yet")
}

func (s *Server) PublishDefinition(ctx context.Context, req *workflowv1.PublishDefinitionRequest) (*workflowv1.PublishDefinitionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "PublishDefinition not implemented yet")
}

func (s *Server) ArchiveDefinition(ctx context.Context, req *workflowv1.ArchiveDefinitionRequest) (*workflowv1.ArchiveDefinitionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ArchiveDefinition not implemented yet")
}

func (s *Server) ListDefinitions(ctx context.Context, req *workflowv1.ListDefinitionsRequest) (*workflowv1.ListDefinitionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListDefinitions not implemented yet")
}

func (s *Server) GetDefinition(ctx context.Context, req *workflowv1.GetDefinitionRequest) (*workflowv1.GetDefinitionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetDefinition not implemented yet")
}

func (s *Server) StartInstance(ctx context.Context, req *workflowv1.StartInstanceRequest) (*workflowv1.StartInstanceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "StartInstance not implemented yet")
}

func (s *Server) GetInstance(ctx context.Context, req *workflowv1.GetInstanceRequest) (*workflowv1.GetInstanceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetInstance not implemented yet")
}

func (s *Server) ListInstances(ctx context.Context, req *workflowv1.ListInstancesRequest) (*workflowv1.ListInstancesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListInstances not implemented yet")
}

func (s *Server) ControlInstance(ctx context.Context, req *workflowv1.ControlInstanceRequest) (*workflowv1.ControlInstanceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControlInstance not implemented yet")
}

func (s *Server) ExportInstances(ctx context.Context, req *workflowv1.ExportInstancesRequest) (*workflowv1.ExportInstancesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ExportInstances not implemented yet")
}
