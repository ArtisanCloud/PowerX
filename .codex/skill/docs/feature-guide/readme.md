# feature-guide（PowerX）使用说明

## 1) 何时使用

当你需要为 PowerX 的某个功能生成“可执行指导文档”时使用。

## 2) 基本调用方式

在对话中直接点名技能：

```text
使用 .codex/skill/docs/feature-guide，为 specs/025-powerx-docker-systemd 生成功能指导文档
```

或显式指定输入与输出：

```text
使用 .codex/skill/docs/feature-guide，
输入 specs/025-powerx-docker-systemd/{spec.md,plan.md,tasks.md,quickstart.md}，
输出 docs/guides/features/025-powerx-docker-systemd/guide.md
```

> 默认约定：若输入是 `specs/<feature-id>/...` 且未指定输出，默认写入  
> `docs/guides/features/<feature-id>/guide.md`。

## 3) PowerX 场景推荐写法

```text
使用 .codex/skill/docs/feature-guide，针对 specs/025-powerx-docker-systemd，
聚焦“系统部署 + 平滑升级 + 日志与备份恢复”链路，
输出一份面向平台管理员与运维的可执行文档。
```

## 4) 是否支持多 use case

支持，并按输入自动判断是否拆分；不是固定数量、不是固定命名。

推荐输出目录：

```text
docs/guides/features/<feature-id>/
  guide.md
  usecase-<slug-a>.md
  usecase-<slug-b>.md
```

## 5) 多 use case 调用示例

```text
使用 .codex/skill/docs/feature-guide，按 specs/025-powerx-docker-systemd 的 user stories 自动拆分 use case 文档
```

```text
仅生成 US1 对应文档，并在 guide.md 中保留索引
```

## 6) 每次调用建议补充的信息

- 目标读者（研发/QA/运维/项目负责人）
- 运行环境（本地/测试/生产）
- 文档输出路径
- 是否要求包含 curl、SQL、回滚步骤
- 是否只覆盖单个 use case

## 7) 质量门槛（简版）

- 文档必须可执行（入口、命令、预期结果、失败处理）
- 必须包含流程图与泳道图
- 必须有代码映射（路由/服务/配置/测试）
- 必须包含验收标准与排障步骤
