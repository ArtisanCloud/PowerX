//go:build ignore

package workflowcontract

import (
	"context"
	"testing"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestWorkflowDefinitionGRPCFlow(t *testing.T) {
	env := testenv.New(t)
	client, cleanup := env.StartGRPCServer()
	defer cleanup()

	ctx := context.Background()
	reqCtx := &commonv1.RequestContext{
		TenantId:  1001,
		MemberId:  42,
		RequestId: "req-grpc-001",
	}

	createResp, err := client.CreateDefinition(ctx, &workflowv1.CreateDefinitionRequest{
		Ctx:         reqCtx,
		Name:        "demo-orchestration",
		Description: "contract test workflow",
		Steps: []*workflowv1.WorkflowStepDefinition{
			{
				StepId:      "prepare",
				DisplayName: "Prepare",
				Type:        workflowv1.StepType_STEP_TYPE_AGENT,
				NextStepIds: []string{"finish"},
			},
			{
				StepId:      "finish",
				DisplayName: "Finish",
				Type:        workflowv1.StepType_STEP_TYPE_SYSTEM,
			},
		},
		Metadata: mustStruct(map[string]any{"owner": "qa"}),
	})
	require.NoError(t, err)
	require.NotNil(t, createResp.GetDefinition())
	require.Equal(t, int32(1), createResp.GetDefinition().GetVersion())

	publishResp, err := client.PublishDefinition(ctx, &workflowv1.PublishDefinitionRequest{
		Ctx:          reqCtx,
		DefinitionId: createResp.GetDefinition().GetDefinitionId(),
	})
	require.NoError(t, err)
	require.Equal(t,
		workflowv1.WorkflowDefinitionStatus_WORKFLOW_DEFINITION_STATUS_PUBLISHED,
		publishResp.GetDefinition().GetStatus(),
	)

	listResp, err := client.ListDefinitions(ctx, &workflowv1.ListDefinitionsRequest{
		Ctx: reqCtx,
	})
	require.NoError(t, err)
	require.NotEmpty(t, listResp.GetDefinitions())

	startResp, err := client.StartInstance(ctx, &workflowv1.StartInstanceRequest{
		Ctx:           reqCtx,
		DefinitionId:  createResp.GetDefinition().GetDefinitionId(),
		Input:         mustStruct(map[string]any{"ticket": "A-1"}),
		CorrelationId: "order-001",
	})
	require.NoError(t, err)
	require.NotNil(t, startResp.GetInstance())
	require.Equal(t,
		workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_RUNNING,
		startResp.GetInstance().GetState(),
	)
}

func mustStruct(v map[string]any) *structpb.Struct {
	if v == nil {
		return nil
	}
	st, err := structpb.NewStruct(v)
	if err != nil {
		panic(err)
	}
	return st
}
