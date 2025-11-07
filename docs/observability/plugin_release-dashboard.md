# Plugin Release Observability Dashboard

This runbook aligns Prometheus metrics emitted by `backend/internal/service/plugin_release/instrumentation`
with Grafana dashboards and alert rules produced in Phase 7.

## Dashboard Sections

1. **Local Hotload Loop**
   - Panels: `plugin_release.hotload.latency_ms` (p50/p95) and session success rate.
   - Guidance: if p95 > 15 minutes, run `px-plugin dev --watch --timeout 5m` against a staging tenant to reproduce.
2. **Release Guardrail**
   - Panels: quality gate pass rate, plan generation latency, and workflow approval timeline.
   - Guidance: a sharp drop in pass-rate triggers a retrospective; fetch candidate metadata via
     `GET /api/admin/plugin-release/candidates/:id`.
3. **Deployment Runtime**
   - Panels: `plugin_release.canary.phase_duration_seconds` stacked per phase and `plugin_release.canary.error_rate`.
   - Guidance: verify gray deployments with `powerx publish deploy --plan-id <id>` and check Event Fabric topics.
4. **Distribution & Marketplace**
   - Panels: `plugin_release.distribution.sla_seconds` histogram and offline import job throughput.
   - Guidance: cross-reference `backend/reports/plugin_release/dry_run.md` and CLI logs (`powerx publish package --offline`).

## Alert Suite

The helper `BuildDefaultAlertSuite` generates three canonical Prometheus rules:

| Alert | Trigger | Severity | Action |
|-------|---------|----------|--------|
| `plugin_release_canary_rollback_sla_breach` | `plugin_release_canary_rollback_total` or rollback duration > SLA | critical | Halt `powerx publish deploy`, inspect metrics + audit |
| `plugin_release_hotload_latency_regression` | p95 hotload latency > 15 min | warning | Ping developer-oncall, check tenant cache + px-plugin logs |
| `plugin_release_offline_import_stuck` | Offline import SLA exceeded | warning | Escalate to Marketplace operations, review listings |

### Exporting Rules

```yaml
groups:
  - name: plugin-release
    rules:
      - alert: {{ .Name }}
        expr: {{ .Expr }}
        for: {{ .Duration }}
        labels:
          severity: {{ .Severity }}
          dashboard: plugin-release
        annotations:
          summary: {{ .Summary }}
          description: {{ .Description }}
```

### Runbook Links

- `docs/use_cases/_from_hub/SCN-PUBLISH-HUB-001/index.md` — high-level scenarios
- `specs/001-install-plugin-pxp/quickstart.md` — engineer quickstart
- `backend/reports/plugin_release/dry_run.md` — latest dry-run artifact
