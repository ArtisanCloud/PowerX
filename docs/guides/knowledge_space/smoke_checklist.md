# Knowledge Space Smoke Checklist

> 用于阶段性冒烟（T058），覆盖 Provisioning → Ingestion → Fusion → Feedback，并包含 Knowledge Update（US6–US9）的关键路径检查。完成后请将结果附加到版本发布说明。

推荐直接运行一键脚本（会生成/覆盖 `docs/releases/knowledge_space_smoke_report.md`）：

```bash
bash scripts/ops/knowledge-space-smoke.sh
```

| 步骤 | 验证项 | 命令 / 链接 | 结果 |
| --- | --- | --- | --- |
| 1 | `go test ./tests/contract/knowledge_space` |  | ☐ |
| 2 | `go test ./tests/integration/knowledge_space` |  | ☐ |
| 3 | `npm run test:unit -- tests/unit/knowledge-spaces/ingestion.spec.ts` |  | ☐ |
| 4 | `npm run test:e2e -- --grep "knowledge-spaces"`（包含 fusion / feedback） |  | ☐ |
| 5 | 手动执行 Quickstart 第 7 步：创建空间→触发入库→发布融合策略→提交反馈 | [quickstart.md](../../specs/011-knowledge-space/quickstart.md) | ☐ |
| 6 | 错误回滚脚本：`node scripts/fusion/rollback_strategy.mjs <space> <strategy>` |  | ☐ |
| 7 | 反馈 API：`curl -sS :8077/api/v1/admin/knowledge-spaces/<space>/feedback ...`（需 `Authorization` + `X-Tenant-UUID`） |  | ☐ |
| 8 | 验证 `reports/_state/knowledge-spaces.json` 既包含 `ingestion` 又包含 `feedback` 段 | `cat reports/_state/knowledge-spaces.json | jq '.'` | ☐ |
| 9 | 验证 `reports/_state/knowledge-update.json` 中 `delta/event/decay/release` 段存在且字段完整 | `cat reports/_state/knowledge-update.json | jq '.'` | ☐ |
| 10 | 验证模块级快照存在：`backend/reports/_state/knowledge-{decay,release}.json` | `ls backend/reports/_state | rg 'knowledge-(decay|release)\\.json'` | ☐ |
| 11 | Grafana Dashboards：`Knowledge Space`、`fusion-pipeline`、`feedback-loop`、`Knowledge Delta Sync`、`Event Hotfix`、`Knowledge Decay Monitor`、`Tenant Release Control` 均无红色告警 |  | ☐ |
| 12 | 将本次日期填入下表，并附上报告路径 | `docs/releases/knowledge_space_smoke_report.md` | ☐ |

## 最近一次冒烟记录

| 日期 | 执行人 | 备注 |
| --- | --- | --- |
| _yyyy-mm-dd_ | _owner@powerx_ | _示例：融合降级演练完成、反馈风暴 60 条_ |

完成后请将表格贴入 release note，并将 Grafana 截图存档到 `docs/releases/<version>/`.
