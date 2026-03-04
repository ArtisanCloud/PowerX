# 报表目录说明（reports/_state vs backend/reports/_state）

本仓库存在两套状态快照目录，分别用于不同层级的产物：

## 1) `backend/reports/_state/`（模块级快照）

- 由后端服务在运行时写入，按模块拆分。
- 典型文件：
  - `backend/reports/_state/knowledge-decay.json`
  - `backend/reports/_state/knowledge-release.json`
  - `backend/reports/_state/knowledge-delta.json`
  - `backend/reports/_state/knowledge-event.json`
  - `backend/reports/_state/knowledge-feedback.json`

## 2) `reports/_state/`（项目级聚合快照）

- 用于跨模块的聚合总览与对外可读的“领导看板/审计快照”。
- 典型文件：
  - `reports/_state/knowledge-update.json`（聚合：delta/feedback/event/decay/release）
  - `reports/_state/knowledge-spaces.json`（按 spaceId 记录 ingestion/feedback 快照，便于排障与周报）
  - `reports/_state/qa-reasoning.json`（默认在此处）

## 约定

- **单模块/单能力写入的快照**：落在 `backend/reports/_state/*`
- **跨模块聚合总览**：落在 `reports/_state/*`

> 若要统一目录，应做一次全仓迁移（含 defaults、deps 注入、脚本、文档与兼容策略），避免线上脚本找不到产物。
