package testenv

import (
	"net"
	"testing"

	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	workflowgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/workflow"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
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
	coremodel.PowerXSchema = "main"
	if err := bootstrapSchema(db); err != nil {
		t.Fatalf("bootstrap workflow schema: %v", err)
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

func bootstrapSchema(db *gorm.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS main.workflow_definitions (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uuid TEXT UNIQUE,
            tenant_id INTEGER NOT NULL,
            name TEXT NOT NULL,
            description TEXT,
            version INTEGER NOT NULL DEFAULT 1,
            status TEXT NOT NULL DEFAULT 'draft',
            step_graph TEXT NOT NULL,
            default_retry_policy TEXT,
            compensation_policy TEXT,
            sla_policy TEXT,
            metadata TEXT,
            created_by TEXT,
            published_at DATETIME,
            archived_at DATETIME,
            last_published_by TEXT,
            last_change_note TEXT,
            version_alias TEXT,
            initial_context_schema TEXT,
            created_at DATETIME,
            updated_at DATETIME,
            deleted_at DATETIME
        );`,
		`CREATE TABLE IF NOT EXISTS main.workflow_instances (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uuid TEXT UNIQUE,
            tenant_id INTEGER NOT NULL,
            definition_uuid TEXT NOT NULL,
            definition_version INTEGER NOT NULL,
            state TEXT NOT NULL,
            input_context TEXT,
            output_context TEXT,
            sla_snapshot TEXT,
            last_error TEXT,
            correlation_id TEXT,
            tags TEXT,
            sla_deadline DATETIME,
            started_at DATETIME,
            completed_at DATETIME,
            current_step_id TEXT,
            last_transition_at DATETIME,
            next_heartbeat_due DATETIME,
            created_at DATETIME,
            updated_at DATETIME,
            deleted_at DATETIME
        );`,
		`CREATE TABLE IF NOT EXISTS main.workflow_step_records (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            instance_uuid TEXT NOT NULL,
            step_id TEXT NOT NULL,
            type TEXT NOT NULL,
            state TEXT NOT NULL,
            subject_type TEXT,
            subject_uuid TEXT,
            tool_grant_id TEXT,
            tool_grant_version INTEGER,
            attempt INTEGER,
            payload_in TEXT,
            payload_out TEXT,
            failure_reason TEXT,
            scheduled_at DATETIME,
            started_at DATETIME,
            completed_at DATETIME,
            last_transition_at DATETIME,
            awaiting_human INTEGER DEFAULT 0,
            created_at DATETIME,
            updated_at DATETIME,
            deleted_at DATETIME
        );`,
		`CREATE TABLE IF NOT EXISTS main.workflow_step_compensations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            step_record_id INTEGER NOT NULL,
            state TEXT NOT NULL,
            handler TEXT NOT NULL,
            initiated_by TEXT,
            notes TEXT,
            started_at DATETIME,
            completed_at DATETIME,
            created_at DATETIME,
            updated_at DATETIME,
            deleted_at DATETIME
        );`,
		`CREATE TABLE IF NOT EXISTS main.workflow_agent_assignments (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            step_record_id INTEGER NOT NULL,
            agent_uuid TEXT NOT NULL,
            status TEXT NOT NULL,
            dispatched_at DATETIME NOT NULL,
            acknowledged_at DATETIME,
            ack_deadline DATETIME,
            completed_at DATETIME,
            last_heartbeat_at DATETIME,
            created_at DATETIME,
            updated_at DATETIME,
            deleted_at DATETIME
        );`,
		`CREATE TABLE IF NOT EXISTS main.workflow_events (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            tenant_id INTEGER,
            instance_uuid TEXT,
            event_type TEXT,
            occurred_at DATETIME,
            actor_type TEXT,
            actor_id TEXT,
            summary TEXT,
            payload TEXT,
            correlation_id TEXT,
            step_record_id INTEGER,
            created_at DATETIME,
            updated_at DATETIME,
            deleted_at DATETIME
        );`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
