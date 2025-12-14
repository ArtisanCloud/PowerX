# Tenant UUID-only GA 发布说明

本文件给出 “租户 UUID-only” GA 版本的 CLI / 文档交付方案，用于满足 T8.8「文档/CLI 发布」要求，确保同一套步骤可复现地完成 goreleaser、npm、Homebrew 以及 Docs 站点的更新。

## 版本组成

| 组件 | 版本 | 渠道 | 备注 |
| --- | --- | --- | --- |
| `px` | 1.8.0 | goreleaser（GitHub Release + tarballs） | 已移除 `--tenant-id` / `PX_TENANT_ID`，`px --help` 仅展示 `tenant_uuid` 相关参数 |
| `px-plugin` | 0.16.0 | npm (`@powerx/px-plugin`) + `npm dist-tag latest` | Bridge SDK 仅广播 `tenant_uuid`，提供 `PX_TENANT_UUID` Fallback 提示 |
| `powerx-bridge-client.js` | 0.9.0 | npm + CDN（docs 静态资源） | iframe 交互规范同步更新，拒绝旧 header |
| Docs 站点 | `main` 最新 | Vercel / 内网镜像 | 附带升级指南、FAQ、Playbook 链接 |

## CLI 发布流程

1. **准备**  
   - 更新 `CHANGELOG.md` 与 `README.md` 中的示例命令，删除 `tenant_id` 文案并强调 `X-Tenant-UUID`。  
   - 确认 `backend/cmd/px/` 下所有命令在 `px --help` 中已隐藏旧 flag，可执行 `go run ./cmd/px --help | rg tenant_id` 验证返回 0。
2. **打包**  
   ```bash
   # goreleaser（依赖 GITHUB_TOKEN）
   goreleaser release --clean --snapshot=false

   # npm（px-plugin + bridge）
   cd powerx-plugin && npm version 0.16.0 && npm publish
   cd web-admin/public && npm version 0.9.0 && npm publish --access public
   ```
3. **Homebrew**  
   - 运行 `scripts/ops/release_homebrew.sh px 1.8.0` 自动生成 formula PR。  
   - 待 CI 通过后合入，记录链接到 `PowerX Release Notes` issue。
4. **验收**  
   - `px version list --tenant-uuid ...`、`px plugin dev watch --tenant-uuid ...`、`px version scan` 等核心命令需在 macOS/Linux 上各执行一次，截图归档到 `reports/tenant-uuid-weekly.md#cli`.  
   - `pnpm dlx @powerx/px-plugin@0.16.0 doctor` 验证 Bridge SDK 仅注入 `X-Tenant-UUID`，请求头中不再出现 `X-Tenant-ID`。

## 文档发布流程

1. **Docs 仓库**  
   - `docs/operations/tenant-uuid-upgrade.md`、`docs/guides/plugin_release/application_runbook.md`、`docs/standards/_shared/cli-install-and-naming.md` 三个入口需同步说明 UUID-only，引用 CLI/SDK 新版本号。  
   - 新增 FAQ：如何排查旧 CLI、如何查看 `tenant_uuid_only_request_total` 监控。
2. **站点部署**  
   - 合并 PR 后触发 Vercel/内部静态站点构建；若 Vercel 环境变量中需要 `NEXT_PUBLIC_TENANT_UUID_ONLY=true`，务必在发布前确认。  
   - 发布完成后在 `#docs-release` 频道贴部署链接与截图。
3. **外部沟通**  
   - `PowerX Release Notes`（GitHub Discussion 或 Issue）追加条目，包含发布日期、CLI/SDK 版本、回滚指引链接。  
   - `#announcements` 与客户 Success 邮件模板引用本文件，并附带 `docs/operations/tenant-uuid-upgrade.md`。

## 检查清单

| 步骤 | 负责人 | 状态 | 记录 |
| --- | --- | --- | --- |
| `px` goreleaser 发布 | DevRel @nova | ☐ | goreleaser run log + GitHub Release 链接 |
| npm (`px-plugin`, `powerx-bridge-client.js`) | DevRel @nova | ☐ | npm publish 截图，`npm info` 输出 |
| Homebrew Formula 更新 | DevEx @ian | ☐ | Homebrew PR 链接 |
| CLI 手动验收 | QA @ian | ☐ | `reports/tenant-uuid-weekly.md#cli` |
| Docs PR 合并 & 部署 | Docs @nova | ☐ | Vercel/job 链接 |
| Release Notes / 公告 | PM @lily | ☐ | Issue + Slack + 邮件模板链接 |

完成以上 checklist 后，在 `tmp/tenant-id-migration-plan.md` 中将 T8.8 对应条目标记为 ✅，并在 `projects/tenant-uuid/approvals.md` 写入签字记录。
