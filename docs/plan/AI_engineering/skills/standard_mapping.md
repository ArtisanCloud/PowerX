# Skill 标准映射规范（SKILL.md -> PowerX）

本文定义外部 `SKILL.md` 与 PowerX 内部 `SkillManifest` 的映射规则。

## 1. 标准策略

1. 首版以 `SKILL.md` 为主输入格式。
2. PowerX 支持在不破坏兼容的前提下扩展字段。
3. 未识别字段进入 `extensions`，不直接丢弃。

## 2. 核心映射表

| SKILL.md 字段 | PowerX 字段 | 必填 | 规则 |
| --- | --- | --- | --- |
| `name` | `skill_id` | 是 | `kebab-case`，全局唯一 |
| `version` | `version` | 是 | 语义化版本，建议 `MAJOR.MINOR.PATCH` |
| `description` | `description` | 是 | 1-300 字 |
| `scope` | `scope` | 否 | `agent/backend/web-admin/ops` 等 |
| `inputs` | `input_schema` | 否 | 归一为 JSON Schema 风格对象 |
| `outputs` | `output_schema` | 否 | 归一为 JSON Schema 风格对象 |
| `dependencies` | `dependencies` | 否 | 工具、运行时依赖 |
| `entrypoints` | `entrypoints` | 是 | 至少一个执行入口 |
| `references` | `references` | 否 | 追溯信息与文档链接 |

## 3. PowerX 扩展字段

PowerX 在 Manifest 增加以下治理字段：

- `source`：`builtin/plugin/third_party`
- `source_uri`：托管地址或镜像地址
- `checksum`：包校验值
- `signature`：签名（可选）
- `tenant_scope`：`global/tenant`
- `visibility`：可见性策略
- `tool_grants`：执行所需授权

## 4. 兼容策略

1. 未带 `version` 的 Skill 拒绝注册。
2. 未带 `entrypoints` 的 Skill 拒绝注册。
3. 字段类型不匹配时，记录校验错误并返回 `invalid_manifest`。
4. 扩展字段必须带 `x_powerx_` 前缀或进入 `extensions`。

## 5. 解析流程

1. 加载 `SKILL.md`。
2. YAML/Markdown Front Matter 解析。
3. 字段标准化（trim/lowercase/case-normalization）。
4. 映射到 `SkillManifest`。
5. 校验必填字段与签名校验策略。
6. 入库前写入 `normalized_manifest` 快照。

## 6. 版本策略

1. 同 `skill_id` 允许多版本并存。
2. 最新发布版由 `status=published` + `is_latest=true` 标记。
3. 回滚时切换 `is_latest` 指向旧版本，不覆盖历史。

## 7. 错误码建议

- `skill.invalid_manifest`
- `skill.unsupported_version`
- `skill.entrypoint_missing`
- `skill.signature_invalid`
- `skill.checksum_mismatch`

