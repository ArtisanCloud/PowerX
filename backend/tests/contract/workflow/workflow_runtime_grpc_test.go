package workflowcontract

import (
	"testing"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestWorkflowRuntimeGRPCContracts(t *testing.T) {
	chdirBackendRoot(t)
	env := testenv.New(t)
	client, cleanup := env.StartGRPCServer()
	defer cleanup()

	ctx := workflowGRPCContext(t, testenv.ContractTenantUUID)
	reqCtx := &commonv1.RequestContext{
		MemberId: 7,
		Attributes: map[string]string{
			"tenant_uuid": testenv.ContractTenantUUID,
		},
	}

	nodeCatalogResp, err := client.ListNodeCatalog(ctx, &workflowv1.ListNodeCatalogRequest{Ctx: reqCtx})
	require.NoError(t, err)
	require.NotEmpty(t, nodeCatalogResp.GetItems())
	assertNoWorkflowTenantLeakProto(t, nodeCatalogResp)

	nodeResp, err := client.GetNodeCatalogItem(ctx, &workflowv1.GetNodeCatalogItemRequest{Ctx: reqCtx, NodeKind: "knowledge.publish"})
	require.NoError(t, err)
	require.Equal(t, "knowledge.publish", nodeResp.GetItem().GetNodeKind())

	createResp, err := client.CreateDefinition(ctx, &workflowv1.CreateDefinitionRequest{
		Ctx:  reqCtx,
		Name: "grpc-runtime",
		Steps: []*workflowv1.WorkflowStepDefinition{{
			StepId:   "input",
			Type:     workflowv1.StepType_STEP_TYPE_SYSTEM,
			NodeKind: "input.capture",
			Config:   mustStructPB(t, map[string]any{}),
		}},
	})
	require.NoError(t, err)
	definitionUUID := createResp.GetDefinition().GetDefinitionUuid()
	require.NotEmpty(t, definitionUUID)

	validateResp, err := client.ValidateDefinition(ctx, &workflowv1.ValidateDefinitionRequest{
		Ctx:            reqCtx,
		DefinitionUuid: definitionUUID,
	})
	require.NoError(t, err)
	require.True(t, validateResp.GetValid())

	publishResp, err := client.PublishDefinition(ctx, &workflowv1.PublishDefinitionRequest{
		Ctx:            reqCtx,
		DefinitionUuid: definitionUUID,
	})
	require.NoError(t, err)
	require.Equal(t, workflowv1.WorkflowDefinitionStatus_WORKFLOW_DEFINITION_STATUS_PUBLISHED, publishResp.GetDefinition().GetStatus())

	startResp, err := client.StartInstance(ctx, &workflowv1.StartInstanceRequest{
		Ctx:            reqCtx,
		DefinitionUuid: definitionUUID,
		Input:          mustStructPB(t, map[string]any{"source": "grpc"}),
	})
	require.NoError(t, err)
	instanceUUID := startResp.GetInstance().GetInstanceUuid()
	require.NotEmpty(t, instanceUUID)

	instanceResp, err := client.GetInstance(ctx, &workflowv1.GetInstanceRequest{
		Ctx:          reqCtx,
		InstanceUuid: instanceUUID,
		IncludeSteps: true,
	})
	require.NoError(t, err)
	require.NotNil(t, instanceResp.GetInstance())

	reviewResp, err := client.ListHumanReviewTasks(ctx, &workflowv1.ListHumanReviewTasksRequest{Ctx: reqCtx})
	require.NoError(t, err)
	require.NotNil(t, reviewResp)

	seedResp, err := client.SeedWorkflowPacks(ctx, &workflowv1.SeedWorkflowPacksRequest{
		Ctx:          reqCtx,
		WorkflowKeys: []string{"marketing_knowledge_capture"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, seedResp.GetPacks())

	packListResp, err := client.ListWorkflowPacks(ctx, &workflowv1.ListWorkflowPacksRequest{Ctx: reqCtx})
	require.NoError(t, err)
	require.NotEmpty(t, packListResp.GetPacks())

	packResp, err := client.GetWorkflowPack(ctx, &workflowv1.GetWorkflowPackRequest{
		Ctx:         reqCtx,
		WorkflowKey: "marketing_knowledge_capture",
	})
	require.NoError(t, err)
	require.Equal(t, "marketing_knowledge_capture", packResp.GetPack().GetWorkflowKey())
}

func mustStructPB(t *testing.T, v map[string]any) *structpb.Struct {
	t.Helper()
	st, err := structpb.NewStruct(v)
	require.NoError(t, err)
	return st
}
