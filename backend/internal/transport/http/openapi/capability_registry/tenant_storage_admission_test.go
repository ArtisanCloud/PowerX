package capability_registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestStorageAdmissionHTTPReturnsMachineCodeAndTraceWithoutPrivateErrors(t *testing.T) {
	for _, code := range []string{"STORAGE_CONTROL_SCHEMA_NOT_READY", "STORAGE_GOVERNANCE_UNAVAILABLE", "STORAGE_CAPACITY_EXHAUSTED"} {
		t.Run(code, func(t *testing.T) {
			private := "password=storage-private SELECT internal FROM secret"
			err := fmt.Errorf("private wrapper: %w", dto.NewErrorWithCode(503, code, private, errors.New(private)))
			invoker := &externalIdentityContractInvoker{err: err}
			response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), iamWSInvocationRequest())
			require.Equal(t, 503, response.Code)
			var body map[string]interface{}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, code, body["error_code"])
			require.Equal(t, code, body["reason_code"])
			require.Equal(t, "trace-iam-typed-error", body["details"].(map[string]interface{})["trace_id"])
			require.NotContains(t, response.Body.String(), "storage-private")
			require.NotContains(t, response.Body.String(), "SELECT")
		})
	}
}
