package bootstrap

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"testing"
	"time"
)

func TestBootstrapForwardsConfiguredAIRequestAndQueueTimeouts(t *testing.T) {
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal([]byte(`ai:
  defaults:
    llm:
      request_timeout: 10m
  runtime:
    queue_wait_timeout: 2m
capability_registry:
  default_http_timeout_seconds: 20
  ai_multimodal_http_timeout_seconds: 310
`), &cfg))
	opts := capabilityRegistryOptions(&cfg)
	require.Equal(t, 10*time.Minute, opts.LLMRequestTimeout)
	require.Equal(t, 2*time.Minute, opts.LLMQueueWaitTimeout)
	require.Equal(t, 310*time.Second, opts.AIMultimodalHTTPTimeout)
	require.Equal(t, 20*time.Second, opts.DefaultHTTPTimeout, "ordinary capability calls retain their own configured timeout")
}
