//go:build ignore

package workflowcontract

import (
	"context"
	"testing"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWorkflowControlGRPC(t *testing.T) {
	env := testenv.New(t)
	client, cleanup := env.StartGRPCServer()
	defer cleanup()

	ctx := context.Background()
	reqCtx := &commonv1.RequestContext{
		TenantId:  1001,
		MemberId:  9001,
		RequestId: "ctrl-grpc-001",
	}

	createResp, err := client.CreateDefinition(ctx, &workflowv1.CreateDefinitionRequest{
		Ctx:         reqCtx,
		Name:        "runtime-control-demo",
		Description: "demo workflow for runtime control contract",
		Steps: []*workflowv1.WorkflowStepDefinition{
			{
				StepId:      "prepare",
				DisplayName: "Prepare",
				Type:        workflowv1.StepType_STEP_TYPE_SYSTEM,
				NextStepIds: []string{"agent_step"},
			},
			{
				StepId:        "agent_step",
				DisplayName:   "Agent Step",
				Type:          workflowv1.StepType_STEP_TYPE_AGENT,
				NextStepIds:   []string{"finalize"},
				Compensatable: true,
				Config: mustStruct(map[string]any{
					"capability": "demo.capability",
					"agent_id":   uuid.New().String(),
				}),
			},
			{
				StepId:      "finalize",
				DisplayName: "Finalize",
				Type:        workflowv1.StepType_STEP_TYPE_SYSTEM,
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, createResp.GetDefinition())

	_, err = client.PublishDefinition(ctx, &workflowv1.PublishDefinitionRequest{
		Ctx:          reqCtx,
		DefinitionId: createResp.GetDefinition().GetDefinitionId(),
	})
	require.NoError(t, err)

	startResp, err := client.StartInstance(ctx, &workflowv1.StartInstanceRequest{
		Ctx:          reqCtx,
		DefinitionId: createResp.GetDefinition().GetDefinitionId(),
		Input: mustStruct(map[string]any{
			"ticket": "RUNTIME-CTRL-1",
		}),
	})
	require.NoError(t, err)
	require.NotNil(t, startResp.GetInstance())

	instanceUUID := uuid.MustParse(startResp.GetInstance().GetInstanceId())
	var agentStep modelworkflow.WorkflowStepRecord
	require.NoError(t, env.DB.WithContext(ctx).
		Where("instance_uuid = ? AND step_id = ?", instanceUUID, "agent_step").
		First(&agentStep).Error)

	require.NoError(t, env.DB.WithContext(ctx).
		Model(&modelworkflow.WorkflowStepRecord{}).
		Where("id = ?", agentStep.ID).
		Updates(map[string]any{
			"state":          "failed",
			"attempt":        1,
			"failure_reason": "temporary outage",
		}).Error)

	require.NoError(t, env.DB.WithContext(ctx).
		Model(&modelworkflow.WorkflowInstance{}).
		Where("uuid = ?", instanceUUID).
		Update("state", "waiting").Error)

	retryResp, err := client.ControlInstance(ctx, &workflowv1.ControlInstanceRequest{
		Ctx:        reqCtx,
		InstanceId: startResp.GetInstance().GetInstanceId(),
		Action:     workflowv1.ControlAction_CONTROL_ACTION_RETRY_STEP,
		StepId:     "agent_step",
	})
	require.NoError(t, err)
	require.NotNil(t, retryResp.GetInstance())
	require.Equal(t,
		workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_RUNNING,
		retryResp.GetInstance().GetState(),
	)

	var stepRecords []modelworkflow.WorkflowStepRecord
	require.NoError(t, env.DB.WithContext(ctx).
		Where("instance_uuid = ?", instanceUUID).
		Order("id ASC").
		Find(&stepRecords).Error)
	require.Len(t, stepRecords, 2)
	require.Equal(t, int32(1), stepRecords[0].Attempt)
	require.Equal(t, "failed", stepRecords[0].State)
	require.Equal(t, int32(2), stepRecords[1].Attempt)
	require.Equal(t, "in_progress", stepRecords[1].State)

	pauseResp, err := client.ControlInstance(ctx, &workflowv1.ControlInstanceRequest{
		Ctx:        reqCtx,
		InstanceId: startResp.GetInstance().GetInstanceId(),
		Action:     workflowv1.ControlAction_CONTROL_ACTION_PAUSE,
		Reason:     "maintenance window",
	})
	require.NoError(t, err)
	require.Equal(t,
		workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_SUSPENDED,
		pauseResp.GetInstance().GetState(),
	)

	resumeResp, err := client.ControlInstance(ctx, &workflowv1.ControlInstanceRequest{
		Ctx:        reqCtx,
		InstanceId: startResp.GetInstance().GetInstanceId(),
		Action:     workflowv1.ControlAction_CONTROL_ACTION_RESUME,
	})
	require.NoError(t, err)
	require.Equal(t,
		workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_RUNNING,
		resumeResp.GetInstance().GetState(),
	)

	listResp, err := client.ListInstances(ctx, &workflowv1.ListInstancesRequest{
		Ctx:          reqCtx,
		IncludeSteps: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, listResp.GetInstances())

	var matched *workflowv1.WorkflowInstance
	for _, inst := range listResp.GetInstances() {
		if inst.GetInstanceId() == startResp.GetInstance().GetInstanceId() {
			matched = inst
			break
		}
	}
	require.NotNil(t, matched)
	require.NotEmpty(t, matched.GetSteps())
	require.GreaterOrEqual(t, len(matched.GetSteps()), 1)
	require.EqualValues(t, 2, matched.GetSteps()[0].GetAttempt())
}
