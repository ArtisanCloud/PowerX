package metadata

import (
	metadatav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/metadata/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"google.golang.org/grpc"
)

type Server struct {
	metadatav1.UnimplementedMetadataGovernanceServiceServer
	deps *shared.Deps
}

func NewServer(deps *shared.Deps) *Server {
	return &Server{deps: deps}
}

func RegisterServer(registrar grpc.ServiceRegistrar, deps *shared.Deps) {
	metadatav1.RegisterMetadataGovernanceServiceServer(registrar, NewServer(deps))
}
