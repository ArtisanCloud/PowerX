# PowerX Plugin Connectors

This directory hosts reference implementations and runbooks for official connector bundles that ship with PowerX. Each connector documents how to:

- Register an instance through the new `/internal/connector-platforms/{platform}/instances` API (and the matching gRPC RPCs).
- Store OAuth tokens + webhook signing keys exclusively via Vault references or sealed secrets.
- Handle callback signature verification and emit trace context back to the Agent Model Hub.
- Implement instance-level pause/resume hooks driven by the Connector Guard service.

## Trace Correlation

All connector invoke/callback flows **must** propagate the PowerX trace context:

| Header | Description |
|--------|-------------|
| `X-PowerX-Trace-ID` | Canonical trace ID from router or upstream task. |
| `traceparent`       | W3C trace context (optional but recommended). |

When issuing outbound HTTP calls (Coze, n8n), forward `X-PowerX-Trace-ID` so downstream systems participate in the same span tree. Callbacks **must** include the same trace ID in response payloads and headers to satisfy SC-003.

## Files

- [`coze/README.md`](./coze/README.md) – Coze connector quickstart (OAuth, webhook signing, trace IDs).
- [`n8n/README.md`](./n8n/README.md) – n8n workspace connector (API key + signed callbacks).

Each README includes:

1. Instance registration payload (HTTP + gRPC) with mapping templates.
2. Vault secret references expected by Connector Guard.
3. Sample webhook verification snippet in TypeScript.
4. Notes for instance-level degradation (auto-pause + resume approvals).

> **Note**: these documents complement the backend implementation under `backend/internal/service/connector_guard` and the contract tests in `backend/tests/contract/{http,grpc}/agent_model_hub/connector_*`. Update both when connector requirements change.
