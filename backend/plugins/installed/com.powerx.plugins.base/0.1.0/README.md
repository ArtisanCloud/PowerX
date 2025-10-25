# PowerX Plugin Lifecycle Resources

Key references for getting a new plugin repository lifecycle-ready:

- Start with the detailed [Lifecycle Overview](docs/lifecycle/overview.md)
- Follow the [Bootstrap Quickstart](docs/lifecycle/quickstart.md) for the abbreviated checklist
- Complete the [Bootstrap Compliance Checklist](docs/lifecycle/checklists/bootstrap-checklist.md)
- Sync docs for reviewers with `make sync-lifecycle-docs`

## Security & Compliance Additions

This plugin ships the privacy, ToolGrant, audit, and vulnerability response capabilities described in `specs/004-security-compliance/`:

- Follow the [Security & Compliance Quickstart](specs/004-security-compliance/quickstart.md) to bootstrap databases, masking rules, and advisory drills.
- Review the [Vulnerability Response Playbook](docs/security/vulnerability-response.md) for intake channels, SLA expectations, and packaging workflows.
- Consult `docs/security/audit-logs.md` for audit export tooling and retention policies.
- Explore the new integration stack (Envelope, Webhooks, Secrets lifecycle) via [docs/security/integration.md](docs/security/integration.md) and the feature [Quickstart](specs/005-protocols-integrations/quickstart.md).

## Protocols & Integrations Final Checks

- Follow the [Feature Quickstart](specs/005-protocols-integrations/quickstart.md) for Envelope drills, webhook replay, and secret rotation walkthroughs.
- Review the final [security guidance](docs/security/integration.md) and [audit checklist](docs/security/audit-logs.md) before enabling production tenants.
- Import the [observability dashboard](docs/observability/integration-dashboard.json) and run the [metrics verification script](scripts/ci/verify_integration_metrics.sh) to confirm SC-001~SC-005 coverage.
- Package the release with `make release` after bumping `plugin.yaml` and updating [release notes](docs/releases/2025-10-integrations.md); use `make integration-smoke` locally or `ci-integration` in CI to wrap formatting、tests、Nuxt build steps.

## Marketplace 商业闭环

- 阅读 [Marketplace 商业闭环指南](docs/overview/marketplace_business_loop.md)，了解 Listing→License→Usage→Revenue 的演练流程。
- 参考 [Sandbox 测试流程](specs/006-marketplace-business/quickstart.md#sandbox-测试流程) 在本地验证 `.pxp` 上架、Usage 上报、Dashboard 告警与分润报表。
- 核对 `backend/etc/config.example.yaml` 中的 `integration.billing.reconciliation` 与 `marketplace` 配置，确保税务、分润与 Usage 告警参数符合需求。

For the full documentation tree, see [docs/readme.md](docs/readme.md).
