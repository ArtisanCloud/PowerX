package skills

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncidentTriageExecutorExecute(t *testing.T) {
	exec := newIncidentTriageExecutor()
	in := ExecuteInput{
		SkillID: "incident-triage",
		Version: "1.0.0",
		Payload: map[string]interface{}{},
		Context: map[string]interface{}{
			"message": "帮我排查 INC-1001，线上接口无法访问，用户大量报错",
		},
	}
	require.True(t, exec.CanHandle(in))

	out, err := exec.Execute(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, "INC-1001", out["incident_id"])
	require.Equal(t, "high", out["severity"])
	require.NotEmpty(t, out["summary"])
	require.NotEmpty(t, out["content"])
}
