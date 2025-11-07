# Plugin Release Security & Configuration Baseline

This document captures the minimal configuration required before enabling the
`plugin_release` module in any environment.

## Feature Flags

| Flag | Expected Value | Notes |
|------|----------------|-------|
| `PLUGIN_RELEASE_ENABLE_LOCAL_INSTALL` | `true` for dev/staging; `false` for prod unless approved | Protects hotload endpoints |
| `PLUGIN_RELEASE_ENABLE_PIPELINE_DEPLOYMENT` | `true` | Guardrail/pipeline must be on before exposing deployment APIs |
| `PLUGIN_RELEASE_ENABLE_OFFLINE_DISTRIBUTION` | `true` only when Marketplace workflows are ready | Prevents rogue offline uploads |

## Runtime Thresholds

- `PluginReleaseOptions.Runtime.RollbackTimeout >= 5m` (auto rollback SLA)
- `PluginReleaseOptions.LocalInstall.MaxArtifactSizeMB <= 512`
- `PluginReleaseOptions.Distribution.EscalationThreshold >= 2`
- `PluginReleaseOptions.Distribution.OfflineBucket`/`OfflinePrefix` must be non-empty

## Observability

- `plugin_release.hotload.latency_ms` exported with alerts targeting 15-minute p95
- `plugin_release.canary.rollback_seconds` emitted with critical alert at breach
- `plugin_release.distribution.sla_seconds` tracked with warning alert at 48h

## Access Control

- HTTP Admin routes must include `AdminOnlyMiddleware`
- gRPC access restricted to authenticated CLI (mTLS or token) via gateway

## Audit

- Audit hooks enabled for `local.install`, `pipeline`, `distribution` events
- CLI and HTTP clients must pass Authorization headers (Bearer or signed token)
