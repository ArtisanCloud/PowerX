# n8n Connector Blueprint

This document adapts the n8n workspace connector to the Agent Model Hub contracts (HTTP + gRPC) and codifies trace/logging guidelines.

## 1. Instance Registration

### HTTP Example

```bash
curl -X POST https://api.powerx.local/internal/connector-platforms/n8n/instances \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "env": "staging",
    "tenantId": "ops-tenant",
    "region": "eu-west-1",
    "oauthRef": "",
    "webhookSigningKeyRef": "vault://n8n/ops/signing",
    "mappingTemplate": {
      "workflow": "incident_automation",
      "fields": {
        "pagerDutyService": "{{task.meta.service}}",
        "priority": "{{task.meta.priority}}"
      }
    },
    "rateLimitPerMinute": 60,
    "secrets": {
      "oauth_token": "",
      "webhook_signing_key": "n8n-signing-secret"
    }
  }'
```

Notes:

- n8n can work with API tokens; when OAuth is not required you may omit `oauthRef` but **must** supply a `webhook_signing_key`.
- Mapping templates use JSON and will be validated for size (<64 KB) and schema.

### gRPC Stub

```proto
client.UpsertConnectorInstance(ctx, &UpsertConnectorInstanceRequest{
  Platform: "n8n",
  Instance: &ConnectorInstanceInput{
    TenantId:            "ops-tenant",
    Region:              "eu-west-1",
    OauthRef:            "",
    WebhookSigningKeyRef:"vault://n8n/ops/signing",
    MappingTemplateJson: "{\"workflow\":\"incident_automation\"}",
    RateLimitPerMinute:  60,
  },
})
```

## 2. Trace Propagation

- Include `X-PowerX-Trace-ID` when triggering n8n workflows via REST.
- Configure n8n’s HTTP Request / Webhook nodes to forward `X-PowerX-Trace-ID` and `traceparent` to any downstream API.
- All n8n callbacks back to PowerX must echo these headers to satisfy observability requirements.

## 3. Webhook Signature Verification

n8n supports a custom header for signatures. Configure the webhook node to send:

| Header | Value |
|--------|-------|
| `X-N8N-Timestamp` | `{{$now}}` |
| `X-N8N-Signature` | `sha256={{hashData("{{ $json }}", secret=SIGNING_KEY)}}` |

Verification in PowerX mirrors Coze (HMAC-SHA256 + 5-minute drift). Sample Go snippet:

```go
func verifyN8nSignature(key, timestamp, body, signature string) error {
    drift := time.Now().UTC().Sub(parseTS(timestamp))
    if math.Abs(drift.Seconds()) > 300 {
        return errors.New("timestamp drift")
    }
    mac := hmac.New(sha256.New, []byte(key))
    mac.Write([]byte(timestamp))
    mac.Write([]byte("."))
    mac.Write([]byte(body))
    expected := hex.EncodeToString(mac.Sum(nil))
    provided := strings.TrimPrefix(strings.ToLower(signature), "sha256=")
    if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
        return errors.New("signature mismatch")
    }
    return nil
}
```

## 4. Error Handling & Auto-Pause

- Report callback failures as `success=false` when invoking `TrackCallbackMetric` (HTTP handler handles this automatically). Connector Guard keeps an EWMA (alpha=0.2) error rate.
- When the error rate exceeds the feature-flag threshold (`connector_guard.auto_pause_threshold`), the instance will be paused and audit entries written (`connector.instance.paused`).
- To resume manually call `POST /connector-platforms/n8n/instances/{instanceId}/pause` with an empty body (resume is handled by Connector Guard service; future endpoints will expose explicit resume).

## 5. Deployment Checklist

- [ ] Signing key stored in Vault and rotated quarterly.
- [ ] Webhook node outputs `X-PowerX-Trace-ID`.
- [ ] Instance registered in staging and production environments separately.
- [ ] Auto-pause alerts mapped to the runbook for the owning team.
