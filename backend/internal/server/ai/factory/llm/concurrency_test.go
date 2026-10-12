package llm

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/stretchr/testify/require"
)

func TestModelConcurrencyLeaseQueuesSameModel(t *testing.T) {
	modelConcurrencyGates.Lock()
	modelConcurrencyGates.items = make(map[string]*modelConcurrencyGate)
	modelConcurrencyGates.Unlock()

	config := &config.ModelConfig{Provider: "ollama", Model: "qwen3:8b", MaxConcurrentRequests: 1}
	releaseFirst, err := acquireModelConcurrencyLease(context.Background(), config)
	require.NoError(t, err)

	acquired := make(chan func(), 1)
	errs := make(chan error, 1)
	go func() {
		release, acquireErr := acquireModelConcurrencyLease(context.Background(), config)
		if acquireErr != nil {
			errs <- acquireErr
			return
		}
		acquired <- release
	}()
	select {
	case <-acquired:
		t.Fatal("second request must wait for the profile lease")
	case <-time.After(25 * time.Millisecond):
	}
	releaseFirst()
	select {
	case releaseSecond := <-acquired:
		releaseSecond()
	case acquireErr := <-errs:
		t.Fatal(acquireErr)
	case <-time.After(time.Second):
		t.Fatal("queued request did not acquire after lease release")
	}
}

func TestModelConcurrencyLeaseIsolatedByModel(t *testing.T) {
	modelConcurrencyGates.Lock()
	modelConcurrencyGates.items = make(map[string]*modelConcurrencyGate)
	modelConcurrencyGates.Unlock()

	first, err := acquireModelConcurrencyLease(context.Background(), &config.ModelConfig{Provider: "ollama", Model: "qwen3:8b", MaxConcurrentRequests: 1})
	require.NoError(t, err)
	defer first()

	second, err := acquireModelConcurrencyLease(context.Background(), &config.ModelConfig{Provider: "openai", Model: "gpt-4o", MaxConcurrentRequests: 1})
	require.NoError(t, err)
	second()
}
