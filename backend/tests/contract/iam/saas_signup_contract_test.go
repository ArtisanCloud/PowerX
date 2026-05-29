package iamcontract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSaaSSignupOpenAPIContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)

	var spec map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	paths, ok := spec["paths"].(map[string]any)
	require.True(t, ok, "openapi paths must exist")

	signupPath, ok := paths["/public/saas/signup"].(map[string]any)
	require.True(t, ok, "saas signup path must exist")
	post, ok := signupPath["post"].(map[string]any)
	require.True(t, ok, "saas signup must be POST")
	require.Equal(t, "signupSaaSTenant", post["operationId"])

	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	req := schemas["SaaSSignupRequest"].(map[string]any)
	required := req["required"].([]any)
	require.NotContains(t, required, "tenant_key")
	require.Contains(t, required, "tenant_name")
	require.Contains(t, required, "owner_password")
	require.NotContains(t, required, "verification_code")
	props := req["properties"].(map[string]any)
	require.Contains(t, props, "verification_code")

	verifyPath, ok := paths["/public/saas/signup/verification-code"].(map[string]any)
	require.True(t, ok, "signup verification code path must exist")
	verifyPost, ok := verifyPath["post"].(map[string]any)
	require.True(t, ok, "signup verification code must be POST")
	require.Equal(t, "sendSaaSSignupVerificationCode", verifyPost["operationId"])
}
