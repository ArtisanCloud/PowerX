package model

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAIEnvironmentAliasesAreLimitedAndKeepCustomCredentialNames(t *testing.T) {
	require.Equal(t, "dev", CanonicalAIEnvironment(" default "))
	require.Equal(t, []string{"dev", "default"}, AIEnvironmentReadOrder("default"))
	for _, env := range []string{"prod", "staging", "preview-demo", ""} {
		require.Equal(t, []string{env}, AIEnvironmentReadOrder(env))
	}
	require.Equal(t, "dev-openai", AICredentialNameForEnvironment("default-openai", "dev", "openai"))
	require.Equal(t, "default-openai-compatible", AICredentialNameForEnvironment("dev-openai-compatible", "default", "openai-compatible"))
	for _, name := range []string{"customer-key", "default-customer-key", "dev-custom"} {
		require.Equal(t, name, AICredentialNameForEnvironment(name, "dev", "openai"))
	}
}
