# 导入第三方 Skill

## 结论

外部 Skill 可以使用 Claude Code/Codex 都能识别的 `SKILL.md` 作为互操作基础；要在 PowerX Team 中执行，必须补充受校验的 PowerX 定义。导入包不会直接被 Runtime 从本地目录或 Git 地址执行。

## 兼容包结构

```text
my-skill/
  SKILL.md
  powerx/manifest.json
  powerx/prompts/zh-CN.md
  powerx/schemas/input.json
  powerx/schemas/output.json
```

`SKILL.md` 的 YAML frontmatter 至少有 `name` 和 `description`。`powerx/` 是可选扩展：没有它时可导入为 `instruction_only`，但不能成为 Team 的可执行节点。

## 保存位置

| 数据 | 保存位置 | Runtime 用途 |
| --- | --- | --- |
| 原始导入压缩包/下载快照 | 当前配置的 Media Storage driver；`skill_package_sources` 保存 URI、checksum、解析元数据。默认 `local` 为 `local://`，明确配置 `s3` 时为 `s3://` | 审计、再次导入；不直接执行 |
| 结构化 Draft 与 Revision | PostgreSQL `skills_definition_drafts`、`skills_definition_revisions` | Runtime 只读取已发布 Revision |
| Canonical 导出包 | 当前配置的 Media Storage driver；Revision 保存 URI、checksum | 下载、迁移、再导入；不直接作为定义来源 |
| 临时 clone/解压目录 | 本地临时空间 | 任务结束后清理；Runtime 不读取 |

## 导入与发布流程

1. 上传或下载用户明确指定的包到临时工作区，解析并做安全检查。
2. 原始内容写入当前配置的 Media Storage driver，记录 `skill_package_sources`。
3. 解析成 `powerx.skill-definition/v2` Draft。`executor.type` 必须是 `llm_prompt`、`capability`、`workflow` 或 `instruction_only`；其中 `llm_prompt` 必须提供 `prompt_template_i18n`，Runtime 按本轮 locale 精确选择。
4. 审核后由发布器生成 Canonical Package，得到 URI 和 SHA256，再把当前 Revision 标记为 `published`。
5. 仅已发布 Revision 可以被 Agent/Team 调用。

## 失败规则

- 不接受 `file://`、本地 `SKILL.md` 路径或远程 Git URL 作为 Runtime 来源。
- 缺 checksum、Media Storage URI、executor 定义或租户权限时明确失败。
- 不能通过“普通聊天”降级执行 `instruction_only` 包。
- 修改已发布 Skill 必须创建新 Revision，不能覆盖历史包或数据库快照。

详细的运行时和验收说明见 [声明式 Skill Runtime](../runtime/declarative-skill-runtime.md)。
