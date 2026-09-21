package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCapabilityRuntimeContract(t *testing.T) {
	contract, err := ParseCapabilityRuntimeContract([]byte(`{"prefer":"tooling","runtime_contract":{"verification_required":true,"side_effect_evidence_schema":"example.operation/v1","retry_max_attempts":1,"alternative_capability_ids":["com.corex.example.alternative"]}}`))
	require.NoError(t, err)
	require.True(t, contract.VerificationRequired)
	require.Equal(t, 1, contract.RetryMaxAttempts)
	require.Equal(t, []string{"com.corex.example.alternative"}, contract.AlternativeCapabilityIDs)
}

func TestParseCapabilityRuntimeContractRejectsUnboundedRetry(t *testing.T) {
	_, err := ParseCapabilityRuntimeContract([]byte(`{"runtime_contract":{"retry_max_attempts":2}}`))
	require.Error(t, err)
}
