package snapshot_ops

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLocalFrozenPackageDoesNotClaimProductionRecovery(t *testing.T) {
	template := `{"protocol":1,"instance_id":"%s","writers_frozen":true,"local_only":%t}`
	require.False(t, supportsManagedRecovery(map[string][]byte{"recovery/contract.json": []byte(fmt.Sprintf(template, uuid.NewString(), true))}))
	require.True(t, supportsManagedRecovery(map[string][]byte{"recovery/contract.json": []byte(fmt.Sprintf(template, uuid.NewString(), false))}))
}
