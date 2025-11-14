package knowledge_space_contract

import (
	"context"
	"net"
	"testing"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestDecayGRPCFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	server := env.GRPCServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go server.Serve(lis)
	t.Cleanup(func() { server.Stop() })

	conn, err := grpc.DialContext(context.Background(), lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client := knowledgev1.NewKnowledgeSpaceAdminServiceClient(conn)
	tpl := env.SeedPolicyTemplate("decay-grpc", "v1")
	space := env.CreateSpaceFixture("decay-grpc-space", tpl)

	runResp, err := client.RunDecayScan(context.Background(), &knowledgev1.RunDecayScanRequest{
		SpaceId:  space.UUID.String(),
		Detected: 2,
	})
	require.NoError(t, err)
	require.Len(t, runResp.GetTasks(), 2)

	listResp, err := client.ListDecayTasks(context.Background(), &knowledgev1.ListDecayTasksRequest{SpaceId: space.UUID.String()})
	require.NoError(t, err)
	require.Len(t, listResp.GetTasks(), 2)

	taskID := listResp.GetTasks()[0].GetTaskId()
	restoreResp, err := client.RestoreDecayTask(context.Background(), &knowledgev1.RestoreDecayTaskRequest{
		TaskId:        taskID,
		Notes:         "false positive",
		FalsePositive: true,
	})
	require.NoError(t, err)
	require.True(t, restoreResp.GetTask().GetFalsePositive())

	listRespAfter, err := client.ListDecayTasks(context.Background(), &knowledgev1.ListDecayTasksRequest{SpaceId: space.UUID.String()})
	require.NoError(t, err)
	require.Len(t, listRespAfter.GetTasks(), 1)

	_, err = client.RestoreDecayTask(context.Background(), &knowledgev1.RestoreDecayTaskRequest{
		TaskId: uuid.New().String(),
	})
	require.Error(t, err)
}
