---
name: crud-index
description: PowerX CRUD 规范聚合入口，按任务类型分派到 http/grpc/migration/model/repository/service 等子技能。
---

# PowerX CRUD Index

## 使用顺序

1) 先判断任务通道：HTTP / gRPC / 前端 Nuxt / STS。
2) 再进入对应原子技能执行（建议优先级）：
- `crud/http`
- `crud/grpc`
- `crud/migration`
- `crud/model`
- `crud/repository`
- `crud/service`
- `crud/dto`
- `crud/handler-http`
- `crud/api-rest`
- `crud/test`
- `crud/transport-grpc`
- `crud/proto-gen`
3) 如涉及管理端前端，联动 `frontend/nuxt/*`。
4) 如涉及鉴权交换，联动 `sts`。

## 验收

- 规则选择与任务边界一致。
- 至少执行一个“通道技能”+ 一个“分层技能”。
