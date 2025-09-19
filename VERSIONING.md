# PowerX gRPC SDK 版本发布指南

本文档说明如何在 PowerX 主仓库中管理 Protobuf 合约与多语言 SDK 的版本，并指导何时、如何打 Tag 与触发 GitHub Actions 的发布流程。

## 目录结构概览

- `api/grpc/contracts/`：Buf 管理的 proto 源文件与生成配置。
- `api/grpc/gen/go/`：Go SDK（独立 Go Module）。
- `api/grpc/sdk/rust/powerx-sdk/`：Rust SDK（Cargo crate）。
- `api/grpc/sdk/php/powerx-sdk/`：PHP SDK。
- `api/grpc/sdk/ts/packages/@powerx/grpc/`：TypeScript SDK（npm 包）。

## 版本号与 Tag 规范

| 类型 | Tag 前缀 | 示例 | 说明 |
|------|---------|------|------|
| Proto 合约 | `proto/` | `proto/v1.3.0` | 表示 Protobuf schema 版本。必须先于各语言 SDK 发布。 |
| Go SDK | `sdk-go/` | `sdk-go/v1.3.0` | Go Module 版本，与 proto 版本保持一致。 |
| Rust SDK | `sdk-rust/` | `sdk-rust/v1.3.0` | Cargo crate 版本。 |
| PHP SDK | `sdk-php/` | `sdk-php/v1.3.0` | PHP SDK 压缩包版本。 |
| TypeScript SDK | `sdk-ts/` | `sdk-ts/v1.3.0` | npm 包版本。 |

> 建议各语言 SDK 的版本号与最新的 `proto/` Tag 保持一致，便于跨语言协同。

## 发布流程

1. **更新 Proto 与生成产物**
   - 修改 `api/grpc/contracts` 下的 `.proto`。
   - 运行 `buf generate`，确保 `api/grpc/gen/*` 目录中所有语言的生成代码同步更新。
   - 提交代码合并到主分支。

2. **发布 Proto 版本**
   - 本地创建 Tag：
     ```bash
     git tag proto/v1.3.0
     git push origin proto/v1.3.0
     ```
   - 推送后会触发 `SDK Release` Workflow 的 `proto` Job：
     - Lint / Breaking 检查
     - 生成最新产物并打包上传至 Release
     - 自动创建名为 “PowerX Proto v1.3.0” 的 GitHub Release

3. **发布各语言 SDK**
   - 确认生成代码已经合并到主分支。
   - 针对每种语言按照下列顺序或并行打 Tag：
     ```bash
     git tag sdk-go/v1.3.0
     git tag sdk-rust/v1.3.0
     git tag sdk-php/v1.3.0
     git tag sdk-ts/v1.3.0
     git push origin sdk-go/v1.3.0 sdk-rust/v1.3.0 sdk-php/v1.3.0 sdk-ts/v1.3.0
     ```
   - 每个 Tag 会触发对应的 Job，执行以下任务：
     - 构建 / 测试语言 SDK
     - 打包产物并上传 GitHub Release（Go/Rust/TS 会生成压缩包或 `.crate`/`.tgz` 文件，PHP 为 `.zip`）
     - 如果配置了凭证（例如 `CRATES_TOKEN`、`NPM_TOKEN`），会自动向生态仓库发布

4. **插件/外部项目使用**
   - 例如 Go 插件可以直接在 `go.mod` 中声明：
     ```go
     require github.com/ArtisanCloud/PowerX/api/grpc/gen/go v1.3.0
     ```
   - 不再需要 `replace` 指向主仓库的相对路径。

## 常见问题

- **什么时候打 Tag？**
  - 必须在相关代码已经合并到主分支之后，由维护者手动创建 Tag 并推送，GitHub Action 会在 Tag 推送后自动执行，Workflow 不会自己新增 Tag。
- **在哪儿打 Tag？**
  - 可在本地使用 `git tag` 然后 `git push origin <tag>`，也可以在 GitHub Release 页面点击 “Draft a new release”，手动选择目标 commit 并输入 Tag 名称。
- **CI 未自动发布到 npm / crates.io？**
  - 确认在仓库 Secrets 中配置了 `NPM_TOKEN`、`CRATES_TOKEN` 等凭证。若未配置，Workflow 会跳过对应的发布步骤，但仍会生成 Release 产物供手动上传。
- **插件发版前如何自检？**
  - 在插件仓库保留 `replace` 进行开发，真正打包前删除 `replace` 并运行 `go mod tidy`，确保能够从远程 Tag 拉取依赖。

## 维护建议

- 在 Repo 中添加 `CHANGELOG` 或者在 Release notes 中记录主要变更，便于使用者了解更新。
- 发布后将 Release 链接同步到内部文档或公告渠道，提醒插件/客户端团队升级。
- 如需回滚版本，务必同时撤回相关 Tag 与包管理器上的版本，以免造成引用混乱。

