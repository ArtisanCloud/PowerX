package eventfabric

import "google.golang.org/grpc"

// Registrar 定义各 gRPC 服务的注册函数。
type Registrar func(server grpc.ServiceRegistrar)

// RegisterServices 依次调用注册函数，便于在引导阶段集中挂载事件骨干的 gRPC API。
func RegisterServices(server grpc.ServiceRegistrar, registrars ...Registrar) {
	for _, register := range registrars {
		if register == nil {
			continue
		}
		register(server)
	}
}
