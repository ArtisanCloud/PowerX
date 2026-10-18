package runtime

import (
	"context"
	"encoding/json"
	"testing"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTaskResultArtifactVerifiesOriginalBytesAndPreservesNumbers(t *testing.T) {
	ctx := context.Background()
	objects := &planMemoryObjects{items: map[string][]byte{}}
	ref := agent_run.TaskRef{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), Revision: 1, TaskID: "report", Attempt: 1}
	const literal = "123456789012345678901234567890.125"
	// The publication companions are typed structs. Their field order differs
	// from a map produced by decoding JSON, as do arbitrary customer tool DTOs.
	result := &aschema.ExecutionResult{Success: true, Data: map[string]any{
		"result": struct {
			Schema string      `json:"schema"`
			Value  json.Number `json:"value"`
		}{Schema: "customer.result/v1", Value: json.Number(literal)},
	}, Metadata: map[string]any{"durable_response_verified": true}}
	key, proof, err := SaveTaskResultArtifact(ctx, objects, ref, result)
	require.NoError(t, err)
	out, err := LoadTaskResultArtifact(ctx, objects, ref, key, proof)
	require.NoError(t, err)
	require.Equal(t, json.Number(literal), out.Data["result"].(map[string]any)["value"])
	require.Equal(t, true, out.Metadata["durable_response_verified"])

	var wire map[string]any
	require.NoError(t, json.Unmarshal(objects.items[key], &wire))
	wire["result"].(map[string]any)["data"].(map[string]any)["result"].(map[string]any)["value"] = "forged"
	objects.items[key], err = json.Marshal(wire)
	require.NoError(t, err)
	_, err = LoadTaskResultArtifact(ctx, objects, ref, key, proof)
	require.ErrorIs(t, err, agent_run.ErrInvalid, "changing stored bytes must still fail")
}
