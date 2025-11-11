# Coze Connector Blueprint

This guide shows how Coze workspaces should integrate with the Agent Model Hub connector guard service. It captures the required payloads, secret handling, trace propagation, and webhook verification logic mandated by FR-010 and SC-003.

## 1. Instance Registration

### HTTP

```bash
curl -X POST https://api.powerx.local/internal/connector-platforms/coze/instances \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "env": "default",
    "tenantId": "demo-tenant",
    "region": "us-east-1",
    "oauthRef": "vault://coze/demo/oauth",            // optional when secrets.oauth_token is provided
    "webhookSigningKeyRef": "vault://coze/demo/webhook",
    "mappingTemplate": {
      "workflow": "sync_leads",
      "fields": {
        "crmTenant": "{{tenantId}}",
        "pipeline": "default"
      }
    },
    "rateLimitPerMinute": 120,
    "secrets": {
      "oauth_token": "coze-refresh-token",
      "webhook_signing_key": "coze-webhook-secret"
    }
  }'
```

### gRPC

```proto
rpc UpsertConnectorInstance(UpsertConnectorInstanceRequest) returns (UpsertConnectorInstanceResponse);

message UpsertConnectorInstanceRequest {
  string platform = 1; // "coze"
  ConnectorInstanceInput instance = 2;
}

ConnectorInstanceInput{
  tenant_id: "demo-tenant",
  region: "us-east-1",
  oauth_ref: "vault://coze/demo/oauth",
  webhook_signing_key_ref: "vault://coze/demo/webhook",
  mapping_template_json: "{\"workflow\":\"sync_leads\"}",
  rate_limit_per_minute: 120
}
```

Connector Guard seals any plaintext secrets and writes them to the `agent_connector_instances` table. Subsequent updates can include `instanceId` to force deterministic UUIDs for CI fixtures.

## 2. Trace & Context Mapping

- Always forward `X-PowerX-Trace-ID` and `traceparent` to Coze when launching flows.
- Include the same headers when Coze calls back into PowerX so the Router can correlate metrics.
- Mapping templates must remain under 64 KB and may reference task attributes via `{{placeholder}}`.

Example mapping JSON:

```json
{
  "workflow": "sync_leads",
  "fields": {
    "crmTenant": "{{tenantId}}",
    "timezone": "{{task.meta.timezone | default: \"UTC\"}}"
  }
}
```

## 3. Webhook Signature Verification

Callbacks from Coze must include:

- `X-Coze-Timestamp` – RFC3339 or Unix millisecond timestamp.
- `X-Coze-Signature` – `sha256=<hex>` HMAC using the connector signing key.
- `X-PowerX-Trace-ID` – forwarded trace context.

Sample verifier (TypeScript):

```ts
import crypto from "node:crypto";

export function verifyCozeSignature(opts: {
  signingKey: string;
  timestamp: string;
  body: string;
  signature: string;
}): void {
  const drift = Math.abs(Date.now() - Date.parse(opts.timestamp));
  if (drift > 5 * 60 * 1000) {
    throw new Error("timestamp drift too large");
  }
  const payload = `${opts.timestamp}.${opts.body}`;
  const expected = crypto.createHmac("sha256", opts.signingKey).update(payload).digest("hex");
  const provided = opts.signature.replace(/^sha256=/i, "");
  if (!crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(provided))) {
    throw new Error("signature mismatch");
  }
}
```

## 4. Auto-Pause & Resume

Connector Guard keeps a rolling error rate per instance. When callback failures exceed the configured `auto_pause_threshold` (see `backend/config/agents/feature_flags/agent-model-hub.yaml`), the instance is paused automatically via `PauseConnectorInstance`.

Operators can resume once the Coze workspace is healthy:

```bash
curl -X POST https://api.powerx.local/internal/connector-platforms/coze/instances/<instanceId>/pause \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"reason":"manual resume"}'
```

Use the Connector Guard audit stream (`connector.instance.*`) to detect pauses/resumes and feed PagerDuty/Grafana dashboards.

## 5. Deployment Checklist

- [ ] OAuth refresh token stored in Vault and rotated every 30 days.
- [ ] Webhook endpoint registered in Coze with signing key matching Connector Guard.
- [ ] Trace headers observed in outbound/inbound logs.
- [ ] Connector instance visible via `GET /internal/connector-platforms/coze/instances` (future list API).
- [ ] Auto-pause alerts wired to Ops channel.
