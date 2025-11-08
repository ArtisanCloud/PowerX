# PowerX gRPC/SDK Release 工作流说明（GitHub Actions）

面向：维护者与发布管理员。目的：解释 `.github/workflows/sdk-release.yml` 的触发方式、执行内容与如何发版。

## 工作流位置与目标
- 文件：`.github/workflows/sdk-release.yml`
- 名称：`SDK Release`
- 目标：通过“打标签”触发多语言 SDK 与 Proto 合约的构建/校验（可选发布）。当前默认以“校验为主，发布步骤留作可选”。

## 触发方式与标签规范
- 触发事件：`push.tags`
- 支持前缀（按语言/类型路由到不同 Job）：
  - `proto/*`（协议 schema 检查与代码生成）
  - `sdk-go/*`
  - `sdk-rust/*`
  - `sdk-php/*`
  - `sdk-ts/*`
- 推荐命名：遵循 SemVer，示例：
  - `proto/v1.2.3`
  - `sdk-go/v1.2.3`
  - `sdk-rust/v1.2.3`
  - `sdk-php/v1.2.3`
  - `sdk-ts/v1.2.3`

注意：这些标签用于路由 CI Job，并不等同于各生态发布所需的“模块路径标签”。若需要 Go Module 等生态级发布，请参见“发布与启用”章节的补充说明。

## 作业（jobs）说明

### proto（当标签以 `proto/` 开头）
- 目录：`api/grpc/contracts`
- 步骤：
  - `actions/checkout@v4`
  - `bufbuild/buf-setup-action@v1`
  - Lint 与兼容性检查：
    - `buf lint`
    - `buf breaking --against '.git#branch=main'`
  - 代码生成：`buf generate`
  - 备注：推送到 BSR（Buf Schema Registry）步骤留空（已注释）。

### sdk-go（当标签以 `sdk-go/` 开头）
- 目录：`api/grpc/gen/go`
- 步骤：
  - `actions/checkout@v4`
  - `go mod tidy`
  - `go build ./...`
- 备注：默认不执行发布；可后续接入 `goreleaser` 或 Go Module 路径标签策略。

### sdk-rust（当标签以 `sdk-rust/` 开头）
- 目录：`api/grpc/sdk/rust/powerx-sdk`
- 步骤：
  - `actions/checkout@v4`
  - `dtolnay/rust-toolchain@stable`
  - `cargo check`
- 可选发布（默认注释）：`cargo publish --token ${{ secrets.CRATES_TOKEN }}`

### sdk-php（当标签以 `sdk-php/` 开头）
- 目录：`api/grpc/sdk/php/powerx-sdk`
- 步骤：
  - `actions/checkout@v4`
  - `ls -la`（占位，建议后续按镜像仓/Packagist 策略发布）
- 建议：使用 subtree-split 镜像到独立仓库，由 Packagist 监听并自动索引发布。

### sdk-ts（当标签以 `sdk-ts/` 开头）
- 目录：`api/grpc/sdk/ts/packages/@powerx/grpc`
- 步骤：
  - `actions/checkout@v4`
  - `actions/setup-node@v4`（Node 20，registry 指向 npmjs）
  - 构建：`npm ci || true`，`npm run build || true`
- 可选发布（默认注释）：
  - `npm publish --access public`
  - 需设置环境：`NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}`

## 目录结构（与工作流相关）
- Proto 源：`api/grpc/contracts/`
- Go 生成产物（Go Module）：`api/grpc/gen/go/`
- SDK 包：
  - Rust：`api/grpc/sdk/rust/powerx-sdk/`
  - PHP：`api/grpc/sdk/php/powerx-sdk/`
  - TS：`api/grpc/sdk/ts/packages/@powerx/grpc/`

## 发布前置与本地自检
在打标签前建议本地先跑与 CI 等价的校验：
- Proto：
  - `cd api/grpc/contracts && buf lint`
  - `cd api/grpc/contracts && buf breaking --against '.git#branch=main'`
  - `cd api/grpc/contracts && buf generate`
- Go：
  - `cd api/grpc/gen/go && go mod tidy && go build ./...`
- Rust：
  - `cd api/grpc/sdk/rust/powerx-sdk && cargo check`
- TS：
  - `cd api/grpc/sdk/ts/packages/@powerx/grpc && npm ci && npm run build`
- PHP：
  - `cd api/grpc/sdk/php/powerx-sdk && ls -la`（或执行你的构建脚本）

## 如何发版（标签驱动）
危险提示：打错标签会触发错误 Job 或误发布。请先确认分支与版本号，必要时先在 Fork/测试仓验证。

- Proto（仅校验与生成，不默认推 BSR）：
  - `git tag proto/v1.2.3 && git push origin proto/v1.2.3`
- Go（默认仅构建校验）：
  - `git tag sdk-go/v1.2.3 && git push origin sdk-go/v1.2.3`
- Rust：
  - 更新 `Cargo.toml` 的 `version`
  - `git tag sdk-rust/v1.2.3 && git push origin sdk-rust/v1.2.3`
- PHP：
  - 如使用 Packagist 镜像仓，按镜像仓策略打 tag：
  - `git tag sdk-php/v1.2.3 && git push origin sdk-php/v1.2.3`
- TS：
  - 更新 `package.json` 的 `version`
  - `git tag sdk-ts/v1.2.3 && git push origin sdk-ts/v1.2.3`

## 启用实际“发布”的方式（可选）
- Proto -> BSR：取消注释 `buf push`，并配置 BSR 凭证（参考 Buf 文档），通常需要 `BUF_TOKEN`/`BUF_USER` 等。
- Rust -> crates.io：取消注释 `cargo publish`，在项目 Secrets 配置 `CRATES_TOKEN`。
- TS -> npm：取消注释 `npm publish` 步骤，并在项目 Secrets 配置 `NPM_TOKEN`。
- PHP -> Packagist：推荐仓库拆分或 subtree 镜像；由 Packagist 钩子自动索引版本，无需在主仓直接 `composer publish`。
- Go -> Module 生态：
  - 当前流程仅构建/校验；若要被 `go get` 以版本方式拉取，需使用“模块路径标签”或接入 Release 工具：
    - 子模块路径打标签：`git tag api/grpc/gen/go/v1.2.3 && git push origin api/grpc/gen/go/v1.2.3`
    - 或接入 `goreleaser`，自动处理 tag、产物与变更日志。

## 常见问题与建议
- Breaking 检查失败：说明与 `main` 比存在不兼容更改。请先走 `deprecate -> remove` 流程或提升 major 版本（并更新 proto 包名至 `v2`）。
- 版本不一致：各语言包的版本号建议保持对齐（便于排查问题）；Rust/TS 需先在 `Cargo.toml`/`package.json` 修改版本再打标签。
- Node 版本：CI 使用 Node 20；本地构建请匹配或高于该版本以避免差异。
- Go Module 标签：若消费者通过 `go get github.com/ArtisanCloud/PowerX/api/grpc/gen/go@vX.Y.Z` 拉取，建议采用“子模块路径标签”或 Release 工具，避免仅 `sdk-go/vX.Y.Z` 的标签导致代理无法解析。
- 安全：不要在仓库提交任何密钥；按照仓库 Secrets 配置发布令牌（`NPM_TOKEN`、`CRATES_TOKEN` 等）。

## 参考
- 工作流文件：`.github/workflows/sdk-release.yml`
- 相关文档：`docs/grpc/sdk/publishing.md`

