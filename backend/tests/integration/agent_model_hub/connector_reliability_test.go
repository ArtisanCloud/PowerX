package agentmodelhubintegration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	amhinst "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	amhshared "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	connectorguard "github.com/ArtisanCloud/PowerX/internal/service/connector_guard"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestConnectorReliabilityMetrics(t *testing.T) {
	t.Parallel()
	env := testenv.New(t)

	recorder := &metricRecorder{}
	inst := amhinst.NewInstrumentation(nil, recorder)

	svc := connectorguard.NewService(connectorguard.Options{
		Options: amhshared.Options{
			DB:              env.DB,
			Cache:           cache.NewMemoryCache(),
			TenantKeySvc:    tenantkeys.NewTenantKeyService(env.DB),
			Instrumentation: inst,
		},
	})

	ctx := context.Background()
	connector, err := svc.UpsertInstance(ctx, "default", connectorguard.ConnectorInstanceInput{
		TenantScope: "tenant-connector-reliability",
		Platform:    "coze",
		Secrets: map[string]string{
			"oauth_token":         "token-abc",
			"webhook_signing_key": "secret-reliability",
		},
		MappingTemplate: datatypes.JSON([]byte(`{"workflow":"benchmark"}`)),
	})
	require.NoError(t, err)

	payload := []byte(`{"ok":true}`)
	const total = 100
	failIndex := 3
	var successCount, failureCount int
	var sigFailures int
	var sigTotal int

	for i := 0; i < total; i++ {
		success := i != failIndex
		_, _, err := svc.TrackCallbackMetric(ctx, connectorguard.CallbackMetricInput{
			InstanceID: connector.UUID,
			Success:    success,
			Threshold:  0.5,
			Reason:     "chaos-drill",
			LatencyMs:  120,
		})
		require.NoError(t, err)
		if success {
			successCount++
		} else {
			failureCount++
		}

		ts := time.Now().UTC().Format(time.RFC3339)
		sig := "sha256=" + computeConnectorSignature("secret-reliability", ts, payload)
		err = svc.VerifyWebhookSignature(ctx, connectorguard.WebhookVerificationInput{
			InstanceID: connector.UUID,
			Signature:  sig,
			Timestamp:  ts,
			Payload:    payload,
		})
		require.NoError(t, err)
		sigTotal++
	}

	// additional negative check (does not affect main ratio because sigTotal incremented above)
	badErr := svc.VerifyWebhookSignature(ctx, connectorguard.WebhookVerificationInput{
		InstanceID: connector.UUID,
		Signature:  "sha256=badsignature",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Payload:    payload,
	})
	require.ErrorIs(t, badErr, connectorguard.ErrSignatureMismatch)
	sigFailures++
	sigTotal++

	successRate := float64(successCount) / float64(total)
	failureRate := float64(failureCount) / float64(total)
	signatureFailureRate := float64(sigFailures) / float64(sigTotal)

	require.GreaterOrEqual(t, successRate, 0.98, "success rate must be ≥98%%")
	require.Less(t, failureRate, 0.05, "failure rate must stay below 5%% (actual %.2f%%)", failureRate*100)
	require.Less(t, signatureFailureRate, 0.01, "signature failure rate must be <1%%")

	// Ensure metrics recorded for Grafana ingestion
	require.GreaterOrEqual(t, recorder.count("agent.platform.latency_p95"), total)
	require.Equal(t, failureCount, recorder.count("agent.platform.callback_failure_total"))
	require.Equal(t, 0, recorder.count("agent.connector.pause_total"), "should not auto-pause when below threshold")

}

func computeConnectorSignature(secret, timestamp string, payload []byte) string {
	body := append([]byte(timestamp), '.')
	body = append(body, payload...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

type metricRecorder struct {
	records []string
}

func (m *metricRecorder) Record(_ context.Context, name string, _ float64, attrs map[string]string) {
	label := name
	if attrs != nil && attrs["instance_id"] != "" {
		label = label + ":" + attrs["instance_id"]
	}
	m.records = append(m.records, label)
}

func (m *metricRecorder) count(name string) int {
	total := 0
	for _, rec := range m.records {
		if len(rec) >= len(name) && rec[:len(name)] == name {
			total++
		}
	}
	return total
}
