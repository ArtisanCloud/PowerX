# Knowledge Space 全链路冒烟报告（T058）

> 用途：供 QA / 发布评审归档。执行清单见 `docs/guides/knowledge_space/smoke_checklist.md`。

## 0. 基本信息

- 日期：_yyyy-mm-dd_
- 执行人：_owner@powerx_
- 分支：_git branch_
- 提交：_git rev-parse --short HEAD_
- 环境：_dev/stage/prod_
- Admin API：`http://127.0.0.1:8077/api/v1/admin`
- Web Admin：`http://127.0.0.1:3030`

## 1. 自动化测试结果

| 项目 | 命令 | 结果 | 备注 |
| --- | --- | --- | --- |
| Backend Contract | `cd backend && go test ./tests/contract/knowledge_space/...` | ☐ PASS / ☐ FAIL / ☐ SKIP |  |
| Backend Integration | `cd backend && go test ./tests/integration/knowledge_space/...` | ☐ PASS / ☐ FAIL / ☐ SKIP |  |
| Web Unit | `cd web-admin && npm run test:unit -- tests/unit/knowledge-spaces/ingestion.spec.ts` | ☐ PASS / ☐ FAIL / ☐ SKIP |  |
| Web E2E | `cd web-admin && npm run test:e2e -- --grep "knowledge-spaces"` | ☐ PASS / ☐ FAIL / ☐ SKIP |  |

## 2. 手动链路确认（Quickstart 第 7 步）

- 空间创建：☐ PASS / ☐ FAIL（含审计徽章、SLA 状态）
- 入库：☐ PASS / ☐ FAIL（完成后自动触发 Corpus Check；blocked/degraded 分支可解释）
- 策略：☐ PASS / ☐ FAIL（L1 场景 → L2 策略包；依赖校验能提示 missing 并给出 remediation）
- Playground：☐ PASS / ☐ FAIL（可检索、可对比 profile）
- 融合：☐ PASS / ☐ FAIL（发布策略、可回滚）
- 反馈：☐ PASS / ☐ FAIL（可提交、SLA 倒计时、可触发 reprocess/rollback）

## 3. 指标 / 告警（5 分钟内可观测）

> 说明：此处以“能触发 + 能恢复”为目标；截图请存档到 `docs/releases/<version>/`。

- `Knowledge Space` 看板无红色告警：☐ PASS / ☐ FAIL
- `fusion-pipeline` 看板无红色告警：☐ PASS / ☐ FAIL
- `feedback-loop` 看板无红色告警：☐ PASS / ☐ FAIL
- `Knowledge Delta Sync` / `Event Hotfix` / `Knowledge Decay Monitor` / `Tenant Release Control` 无红色告警：☐ PASS / ☐ FAIL
- 关键告警触发与恢复（<5m）：☐ PASS / ☐ FAIL

截图/链接：
- _Grafana 截图路径或链接_

## 4. 报表与审计完整性

- `reports/_state/knowledge-spaces.json`（ingestion + feedback 段）存在且更新：☐ PASS / ☐ FAIL
- `reports/_state/knowledge-update.json`（delta/event/decay/release 段）存在且字段完整：☐ PASS / ☐ FAIL
- `backend/reports/_state/knowledge-{decay,release}.json` 存在：☐ PASS / ☐ FAIL
- 审计：相关动作（create/ingest/fusion/feedback/release 等）在审计表中可追踪：☐ PASS / ☐ FAIL

验证输出（可粘贴关键片段）：

```text
<paste outputs here>
```

## 5. 异常与处理记录

- _问题：_
- _影响范围：_
- _修复/规避：_
- _是否阻塞发布：_

