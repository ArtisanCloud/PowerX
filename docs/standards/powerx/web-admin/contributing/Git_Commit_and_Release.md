# Git 提交与发布规范

> 规范 PowerX Web Admin 的 Git 提交消息、分支命名、版本发布与变更日志流程，便于协作与追踪。

---

## 1. 提交信息格式

- 采用 **Conventional Commits**：
  ```
  <type>(scope): <subject>
  ```
  示例：
  - `feat(agent): support multi-session pin`
  - `fix(workflow): prevent node duplication`
  - `docs(environment): update mock guide`
- 常用类型：`feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`, `build`, `ci`.
- `scope` 可选，推荐使用模块名（`agent`, `workflow`, `dashboard`, `env`）。  
- `subject` 使用祈使句，首字母小写，不以句号结尾。

---

## 2. 提交建议

- 保持提交原子化：单一功能或修复。  
- 提交前运行 `npm run lint`、必要时 `npm run build` 验证。  
- 若包含大文件或生成文件，考虑拆分或使用 `.gitattributes`。  
- 对于文档/配置改动，可使用 `docs:`、`chore:` 类型。

---

## 3. 分支策略

- `main`：生产稳定分支。  
- `develop`（可选）：集成环境。  
- 功能分支：`feat/agent-session-stream`, `fix/dashboard-chart`.  
- 提交 PR 前从目标分支拉取最新代码并 rebase。

---

## 4. 版本与发布

- 版本号遵循 SemVer：`MAJOR.MINOR.PATCH`.  
- 发布流程（示例）：
  1. 合并发布分支到 `main`。  
  2. 运行 `npm run build`，确认产物。  
  3. 更新 `CHANGELOG.md`（可使用 `conventional-changelog`）。  
  4. 创建 Tag：`git tag -a v1.2.0 -m "Release 1.2.0"`，推送 `git push --tags`.  
  5. 触发 CI/CD 部署生产环境。  
  6. 将版本号写入 `NUXT_APP_VERSION` / Sentry Release。

---

## 5. 变更日志

- 使用自动生成工具：
  ```bash
  npx conventional-changelog -p angular -i CHANGELOG.md -s
  ```
- 记录功能、修复、Breaking Changes。  
- 在 PR 描述中列出用户影响、测试结果、截图/录屏。

---

## 6. Hotfix 流程

- 基于 `main` 创建分支：`hotfix/fix-crash`.  
- 修复后直接合并到 `main` 与 `develop`（若存在）。  
- 发版采用补丁版本（`1.2.3` → `1.2.4`）。  
- 记录原因和回溯步骤，更新文档/监控。

---

## 7. Review Checklist

- [ ] 提交信息符合格式，并描述实际内容。  
- [ ] PR 描述包含用户影响、测试结果、截图（UI 变更）。  
- [ ] 是否需要更新文档/环境变量。  
- [ ] 是否关联 Tracking Issue/Jira 任务。  
- [ ] 发布后是否通知相关团队（产品、QA、DevOps）。

---

## 8. 后续计划

- 自动化版本号和 Changelog 生成（Semantic Release）。  
- 在 CI 中验证 Conventional Commit 格式（`commitlint`）。  
- 建立发布模板（版本说明、主要更新、回滚指南）。  
- 将版本信息同步到监控与客服系统，便于追踪。
