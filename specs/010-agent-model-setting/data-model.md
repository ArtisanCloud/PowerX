# Data Model – Agent Model Hub Connectivity & Governance

## ProviderProfile
| Field | Type | Description | Validation |
|-------|------|-------------|------------|
| id | UUID | Provider identifier | Required, immutable |
| name | string | Display name | 3-100 chars, unique per tenant scope |
| capabilities | []string | Tags (LLM, VLM, TTS, Embedding) | Must match registered capability catalog |
| primary_endpoint | string | Base URL | HTTPS only |
| regions | []string | Enabled regions | Must exist in region registry |
| tenant_whitelist | []TenantRef | Allowed tenants/envs | Non-empty before publish |
| secret_refs | map[string]string | Vault reference IDs per credential | All credentials must have `vault://` prefix |
| health_score | float | Latest validator score | 0-1 range, computed hourly |
| rollout_status | enum | draft, validating, gray, live, rolled_back | Transition rules below |
| audit_trail_id | string | Link to validation + approval artifacts | Required from validating onwards |

**State transitions**: `draft -> validating -> gray -> live`; any failure sends to `rolled_back` and revives previous live config.

## RoutingPolicy
| Field | Type | Description | Validation |
|-------|------|-------------|------------|
| policy_id | UUID | Unique policy record | Required |
| tenant_scope | string | Tenant or BU identifier | Required |
| version | int | Monotonic version | Auto-increment per tenant |
| rules | []Rule | Weighted provider selections | Each rule references valid ProviderProfile |
| fallback_chain | []ProviderRef | Ordered fallback list | Non-empty |
| approval_record | ApprovalMeta | Who/when approved | Required before deployment |
| safe_mode_thresholds | struct | Hit rate, latency, error thresholds | Provide defaults per spec |
| status | enum | draft, staged, active, rolled_back | Active only after approvals |

**Relationships**: depends on ProviderProfile; telemetry writes results back referencing `policy_id`.

## ConnectorInstance
| Field | Type | Description | Validation |
|-------|------|-------------|------------|
| instance_id | UUID | Connector workspace id | Required |
| platform | enum | coze, n8n, other | Required |
| tenant_scope | string | Tenant/BU owner | Required |
| oauth_ref | string | Vault token reference | Required |
| webhook_signing_key | string | HMAC key reference | Required |
| mapping_template | JSON | Task context mapping rules | JSON schema validated |
| status | enum | active, paused, degrading | Only instance scope, no global |
| error_rate | float | Rolling failure percentage | Auto-managed |
| last_pause_reason | string | Audit note | Required when status != active |

## CostQuotaLedger
| Field | Type | Description | Validation |
|-------|------|-------------|------------|
| ledger_id | UUID | Primary key | Required |
| tenant_id | string | Tenant | Required |
| provider_id | UUID | Provider reference | Optional (tenant-wide budgets) |
| budget_period | enum | daily, weekly, monthly | Required |
| quota_limit | decimal | Monetary or token limit | >0 |
| usage_actual | decimal | Latest usage | Auto-updated from metering |
| anomaly_state | struct | Flags if breach detected | Contains timestamp + type |
| enforcement_state | struct | Pending/confirmed actions | Tracks operator decisions |
| dashboard_scope | string | Controls tenant read-only visibility | Required |

## Shared References
- **TenantRef**: `{ tenant_id, environment }` with validation against IAM multi-tenant registry.
- **ProviderRef**: `{ provider_id, weight }` aligning to ProviderProfile records.
- **ApprovalMeta**: `{ approvers[], outcome, timestamp, notes }`, outcome stored even when BU chooses custom workflows.

## Relationships Overview
- ProviderProfile 1..* RoutingPolicy (policies reference providers)
- ConnectorInstance ↔ Tenant (1..*) with optional ProviderProfile dependency for context mapping
- CostQuotaLedger references ProviderProfile (optional) and Tenant (mandatory)
- Telemetry tables reference ProviderProfile, RoutingPolicy, ConnectorInstance, and CostQuotaLedger for correlation
