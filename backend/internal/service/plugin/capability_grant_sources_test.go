package plugin

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestManifestGrantReconciliationRemovesEmptyAndRetainsIndependentSource(t *testing.T) {
	const read = "com.corex.iam.members.read"
	const manage = "com.corex.agent.session.manage"
	doc := map[string]any{
		"allowed_capabilities":          []string{read, manage},
		"manifest_capabilities":         []string{read, manage},
		"independent_capability_grants": map[string]string{read: "65a6d317-febf-455c-92ac-bc2ff695ee03"},
	}
	approved := map[string]string{read: "65a6d317-febf-455c-92ac-bc2ff695ee03"}
	require.NoError(t, reconcileManifestCapabilityGrants(doc, nil, approved))
	require.Equal(t, []string{read}, doc["allowed_capabilities"])
	require.Empty(t, doc["manifest_capabilities"])
	before, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, reconcileManifestCapabilityGrants(doc, nil, approved))
	after, err := json.Marshal(doc)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestManifestGrantReconciliationDoesNotTrustUnattributedHistory(t *testing.T) {
	doc := map[string]any{"allowed_capabilities": []string{"com.corex.agent.session.manage"}}
	require.NoError(t, reconcileManifestCapabilityGrants(doc, nil, nil))
	require.Empty(t, doc["allowed_capabilities"])
	require.Equal(t, []string{"com.corex.agent.session.manage"}, doc["unattributed_capabilities"])
}

func TestManifestGrantReconciliationDoesNotTrustSelfAttributedSource(t *testing.T) {
	doc := map[string]any{
		"allowed_capabilities":          []string{"com.corex.agent.session.manage"},
		"independent_capability_grants": map[string]string{"com.corex.agent.session.manage": "65a6d317-febf-455c-92ac-bc2ff695ee03"},
	}
	require.NoError(t, reconcileManifestCapabilityGrants(doc, nil, nil))
	require.Empty(t, doc["allowed_capabilities"])
	require.Empty(t, doc["independent_capability_grants"])
	require.Equal(t, []string{"com.corex.agent.session.manage"}, doc["unattributed_capabilities"])
}
