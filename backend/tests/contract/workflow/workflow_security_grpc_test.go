//go:build ignore

package workflowcontract

import (
	"context"
	"testing"

	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/stretchr/testify/require"
)

func TestWorkflowGRPCTenantValidation(t *testing.T) {
	env := testenv.New(t)
	client, cleanup := env.StartGRPCServer()
	defer cleanup()

	_, err := client.CreateDefinition(context.Background(), &workflowv1.CreateDefinitionRequest{
		Name: "invalid",
	})
	require.Error(t, err)
}
