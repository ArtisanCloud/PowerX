package shared

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func TestAIProxyTimeoutCoversConfiguredQueueAndInference(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts CapabilityRegistryOptions
		want time.Duration
	}{
		{"ten-minute-config-with-old-310-second-proxy", CapabilityRegistryOptions{AIMultimodalHTTPTimeout: 310 * time.Second, LLMRequestTimeout: 10 * time.Minute, LLMQueueWaitTimeout: 10 * time.Minute}, 20*time.Minute + 10*time.Second},
		{"five-minute-inference", CapabilityRegistryOptions{AIMultimodalHTTPTimeout: 310 * time.Second, LLMRequestTimeout: 5 * time.Minute, LLMQueueWaitTimeout: time.Minute}, 6*time.Minute + 10*time.Second},
		{"longer-explicit-proxy-is-preserved", CapabilityRegistryOptions{AIMultimodalHTTPTimeout: time.Hour, LLMRequestTimeout: 10 * time.Minute, LLMQueueWaitTimeout: time.Minute}, time.Hour},
		{"unset-values-use-ai-config-defaults", CapabilityRegistryOptions{}, 15*time.Minute + 10*time.Second},
		{"large-config-never-overflows-into-unlimited-http", CapabilityRegistryOptions{LLMRequestTimeout: time.Duration(1<<63 - 1), LLMQueueWaitTimeout: time.Minute}, time.Duration(1<<63 - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, resolveAIMultimodalHTTPTimeout(tc.opts)) })
	}
}

type timeoutRoundTripper func(*http.Request) (*http.Response, error)

func (f timeoutRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAlignedAIProxyStillHonorsEarlierCallerDeadline(t *testing.T) {
	opts := CapabilityRegistryOptions{AIMultimodalHTTPTimeout: 310 * time.Second, LLMRequestTimeout: 10 * time.Minute, LLMQueueWaitTimeout: 10 * time.Minute}
	caller, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	deadline, _ := caller.Deadline()
	sentinel := errors.New("fixture-completed")
	client := http.Client{Timeout: resolveAIMultimodalHTTPTimeout(opts), Transport: timeoutRoundTripper(func(r *http.Request) (*http.Response, error) {
		actual, ok := r.Context().Deadline()
		require.True(t, ok)
		require.Equal(t, deadline, actual, "Core must not extend the plugin/caller deadline")
		return nil, sentinel
	})}
	req, err := http.NewRequestWithContext(caller, http.MethodGet, "http://fixture.invalid", nil)
	require.NoError(t, err)
	_, err = client.Do(req)
	require.ErrorIs(t, err, sentinel)
}

func TestAlignedAIProxySuppliesConfiguredCombinedBudget(t *testing.T) {
	opts := CapabilityRegistryOptions{AIMultimodalHTTPTimeout: 310 * time.Second, LLMRequestTimeout: 10 * time.Minute, LLMQueueWaitTimeout: 10 * time.Minute}
	expected := 20*time.Minute + 10*time.Second
	client := http.Client{Timeout: resolveAIMultimodalHTTPTimeout(opts), Transport: timeoutRoundTripper(func(r *http.Request) (*http.Response, error) {
		actual, ok := r.Context().Deadline()
		require.True(t, ok)
		require.InDelta(t, expected.Seconds(), time.Until(actual).Seconds(), 1)
		return nil, errors.New("fixture-completed")
	})}
	req, err := http.NewRequest(http.MethodGet, "http://fixture.invalid", nil)
	require.NoError(t, err)
	_, err = client.Do(req)
	require.Error(t, err)
}

func TestAIProxyBudgetDoesNotExpandNonLLMHandlerTimeout(t *testing.T) {
	opts := CapabilityRegistryOptions{AIMultimodalHTTPTimeout: 310 * time.Second, LLMRequestTimeout: 10 * time.Minute, LLMQueueWaitTimeout: 10 * time.Minute}
	require.Equal(t, 20*time.Minute+10*time.Second, resolveAIMultimodalHTTPTimeout(opts))
	require.Equal(t, 310*time.Second, resolveAIHandlerTimeout(opts))
}
