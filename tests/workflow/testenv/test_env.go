package testenv

import (
	"net"
	"testing"

	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	workflowgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/workflow"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestEnv 提供工作流特性测试所需依赖。
type TestEnv struct {
	T       *testing.T
	DB      *gorm.DB
	Service *workflowsvc.Service
}

// New 构建内存数据库 + 工作流服务实例。
func New(t *testing.T) *TestEnv {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.MigrateCoreModels(db); err != nil {
		t.Fatalf("migrate core models: %v", err)
	}

	svc := workflowsvc.NewService(db, workflowsvc.ServiceOptions{})

	return &TestEnv{
		T:       t,
		DB:      db,
		Service: svc,
	}
}

// StartGRPCServer 启动仅包含 WorkflowService 的 gRPC Server，返回连接客户端与回收函数。
func (env *TestEnv) StartGRPCServer() (workflowv1.WorkflowServiceClient, func()) {
	env.T.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		env.T.Fatalf("listen grpc: %v", err)
	}
	server := grpc.NewServer()
	workflowv1.RegisterWorkflowServiceServer(server, workflowgrpc.NewServer(env.Service))

	go func() {
		if err := server.Serve(lis); err != nil {
			env.T.Logf("grpc server stopped: %v", err)
		}
	}()

	conn, err := grpc.Dial(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		env.T.Fatalf("dial grpc: %v", err)
	}

	client := workflowv1.NewWorkflowServiceClient(conn)
	cleanup := func() {
		_ = conn.Close()
		server.GracefulStop()
	}
	return client, cleanup
}

// OverrideService 允许测试自定义 Service 依赖。
func (env *TestEnv) OverrideService(opts workflowsvc.ServiceOptions) {
	env.T.Helper()
	env.Service = workflowsvc.NewService(env.DB, opts)
}
