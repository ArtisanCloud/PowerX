# 下游仓库 `docs/standards/**` 只读治理指引

> 目的：确保由 **PowerXDocs** 分发的标准文档，在下游仓库（PowerX、PowerXPlugin、PowerXMarketplace 等）保持只读。此指引可随 `npm run publish:standards` 一并推送，供各仓实施权限与流程约束。

---

## 1. 原则说明

- `docs/standards/**` 的唯一编辑入口是 PowerXDocs 仓库；其它仓库仅接受同步分发。
- 下游仓的任何手动改动都应在下一轮同步中被发现并阻断。
- 自动化分发分支命名统一为 `docs/hub/standards-*`，便于规则识别。

---

## 2. CODEOWNERS 配置

在下游仓根目录创建或追加 `.github/CODEOWNERS`：

```
# 下游仓库示例
docs/standards/**  @PowerXDocs/standards-stewards
```

- 需在仓库设置中开启 “Require review from Code Owners”。
- 建议将分发脚本使用的 bot 或自动化账号也纳入该团队，避免每次同步被阻塞。

---

## 3. 分支保护策略

在下游仓设置默认分支（如 `main` 或 `dev/docs`）的保护规则：

- 禁止直接 push（require pull request）。
- 必须满足检查（见下一节 CI 校验）。
- 要求状态检查通过、Code Owners 审核完成。
- 可选：限制允许合并的分支前缀，例如仅允许 `docs/hub/standards-*`。

---

## 4. CI 校验（示例 GitHub Actions）

在 `.github/workflows/verify-standards-readonly.yml` 中加入检测逻辑：

```yaml
name: Verify Standards Readonly

on:
  pull_request:
    paths:
      - 'docs/standards/**'

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 2

      - name: Ensure branch is from PowerXDocs workflow
        run: |
          if [[ "${GITHUB_HEAD_REF}" != docs/hub/standards/* ]]; then
            echo "docs/standards/** can only be updated via PowerXDocs distribution branches."
            exit 1
          fi

      - name: Detect manual edits
        run: |
          git diff --name-status origin/${GITHUB_BASE_REF}...HEAD | grep '^M\sdocs/standards/' && \
            echo "Detected manual edits; ensure they originate from PowerXDocs." || true
```

要点：

- 仅允许来自 `docs/hub/standards-*` 的 PR。
- 检测到手动修改时直接失败或提示责任团队复核。
- 可配合更严格的 diff 校验（例如比较文件哈希或校验签名）。

---

## 5. 本地开发约束

- 下游仓贡献指南中注明：`docs/standards/**` 为只读目录，不接受手动提交。
- 可在 `package.json` / `Makefile` 中添加 pre-commit 钩子，阻止该目录的 lint/format 改动。
- 若出现紧急修复需求，应回到 PowerXDocs 仓修订后再同步。

---

## 6. 监控与审计

- `publish:standards` 工作流会在 `PowerXDocs/reports/standards/` 生成分发记录，与下游仓 PR ID 对应。
- 建议在下游仓配置仓库通知或 Slack Webhook，一旦 `docs/standards/**` 有 PR 即提醒标准 Steward。
- 可扩展 audit 脚本，对比下游仓历史提交，确保只有同步分支对该目录写入。

---

## 7. 快速检查清单

| 项目 | 是否完成 |
|------|----------|
| CODEOWNERS 指向标准 Steward 团队 |
| 默认分支启用保护，禁止直推 |
| GitHub Actions 或其他 CI 校验分支前缀 |
| 贡献指南注明只读规则 |
| 本地 pre-commit（可选）阻止手动改动 |
| 分发报告与通知机制就绪 |

---

> **提醒**：实施上述措施后，再执行 `npm run publish:standards`，即可把本指引同步到下游仓，帮助团队按照统一流程维护只读规范。若需要更强约束（如签名验证、哈希校验），可结合企业内部合规要求进一步扩展。 
