package plugin_release

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"google.golang.org/grpc"
)

// RegisterServer wires plugin release gRPC handlers. Placeholder until proto definitions land.
func RegisterServer(server grpc.ServiceRegistrar, deps *shared.Deps) {
	// TODO: register plugin release gRPC services once proto contracts are defined.
	_ = deps
	_ = server
}
