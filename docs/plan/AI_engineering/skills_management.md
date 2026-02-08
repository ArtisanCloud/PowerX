# Skills 管理与使用规范

本文档用于约定「Skills」的维护与使用方式，供后续智能体/任务执行时按需加载与复用。

## 目标

- 让任务执行时可声明所需 Skill，避免重复造轮子。
- 支持内置 Skill 与外部来源 Skill（例如 GitHub 列表）。
- 规范化 Skill 的元信息、安装、升级、回滚与使用流程。

## Skill 定义（建议元信息字段）

每个 Skill 以单个目录为单位，至少包含 `SKILL.md`，建议包含以下元信息：

- name：唯一标识（短名，kebab-case）
- version：语义化版本
- description：一句话说明用途
- scope：适用范围（backend/web-admin/ops/agent 等）
- inputs：必需输入
- outputs：输出形态（文档/代码/命令/配置）
- dependencies：运行/工具依赖（如需要 `rg`、`go`）
- entrypoints：主要使用流程或脚本入口
- references：关联资料或仓库链接

## Skill 来源与安装

### 1) 内置 Skill

- 存放路径建议：`skills/<name>/`
- 由仓库维护，随版本发布。

### 2) 外部 Skill（示例来源）

- 允许从标准化 Skill 仓库安装，例如：
  - `https://github.com/VoltAgent/awesome-openclaw-skills`

安装流程建议：
1. 选择 Skill（name/version）
2. 拉取并校验（签名/校验和可选）
3. 安装到 `skills/` 或受控目录
4. 记录安装来源与版本

## 使用方式（任务声明）

执行任务前，允许在任务描述中显式声明所需 Skill：

```
需要技能：skill-a, skill-b
```

智能体按声明加载 `SKILL.md`，遵循其中步骤；如缺失或不可用，应回退到手工流程并记录原因。

## 版本与回滚

- 版本升级需记录变更摘要与兼容性说明。
- 回滚应保留最近 1 个稳定版本。

## 安全与合规

- 禁止自动执行具有破坏性的命令（除非明确授权）。
- 外部 Skill 必须可追溯来源。
- Skill 内如含脚本，应标注权限需求与副作用。

## 后续计划（待实现）

- 提供可视化 Skill 管理页（列表/安装/卸载/升级）。
- 支持一键拉取外部 Skill 清单并筛选安装。
