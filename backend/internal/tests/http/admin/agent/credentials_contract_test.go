package agenthttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type credential struct {
	Data map[string]any `json:"data"`
}

type credentialListResponse struct {
	Credentials []credential `json:"credentials"`
}

func TestAgentCredentialListRedactsSecrets(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("CONTRACT_BASE_URL"))
	if baseURL == "" {
		t.Skip("CONTRACT_BASE_URL not set; skipping HTTP credential contract test")
	}

	req, err := http.NewRequest(
		http.MethodGet,
		fmt.Sprintf("%s/api/admin/agents/settings/credentials?env=default", baseURL),
		nil,
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload credentialListResponse
	err = json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)

	for _, cred := range payload.Credentials {
		if cred.Data == nil {
			continue
		}
		require.NotContains(t, cred.Data, "api_key")
		require.NotContains(t, cred.Data, "secret")
		require.NotContains(t, cred.Data, "client_secret")
		require.NotContains(t, cred.Data, "access_token")
	}
}
