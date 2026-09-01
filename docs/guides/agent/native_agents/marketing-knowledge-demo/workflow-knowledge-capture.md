# Workflow：营销知识采集与发布

本用例验证团队 Demo 之后的业务闭环：草稿审核和知识库发布。

## 操作

1. 打开 `/workflow`，选择“营销知识采集”或“活动复盘沉淀”。
2. 选择目标 Knowledge Space，输入文本材料并运行测试。
3. 在审核任务中通过或拒绝草稿。
4. 通过后在目标知识库确认新增知识或版本。

## 输入建议

提供活动目标、渠道、预算、曝光、点击、线索、SQL、成交、素材说明、已观察问题和业务约束。没有的数据必须明确标为未知。

## 验收与回滚

- 通过：生成草稿、出现审核任务、审核后知识库有增量。
- 失败：在 Workflow 详情定位失败节点输入输出；不要直接改数据库删除知识版本。
- 回滚：拒绝草稿或按知识库版本与审计流程回退。

## 本地验证

```bash
cd backend
GOTOOLCHAIN=go1.26.7 go test ./cmd/database/seed ./internal/service/skills -count=1
```

Workflow 定义位于 `backend/config/workflow_packs/marketing/`，营销 Agent 与 Skill seed 位于 `backend/cmd/database/seed/`。
