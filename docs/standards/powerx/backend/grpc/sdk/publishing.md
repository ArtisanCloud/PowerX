# PowerX API/SDK 发布规范（协议优先）

面向：PowerX 核心服务与插件生态的维护者、SDK 维护者。

目标：只维护“一份 proto 契约”，由官方流水线生成并发布“多语言 SDK 包”（Go/Rust/PHP/TS…）。插件通过包管理器依赖 SDK，禁止在各插件仓库复制粘贴 proto 或自行分叉生成代码（除临时过渡）。

## 核心原则
- 契约统一：集中管理 proto（建议独立仓库，或主仓 `api/grpc/contracts/`）。
- 协议优先：以 proto 为事实来源，CI 负责 lint + 兼容性检查 + 多语言代码生成与发布。
- 版本与兼容：使用 SemVer；严格 `deprecate -> remove` 窗口；CI 做 breaking 检查。
- 开发体验：反射仅用于调试/探索，不作为生产客户端方案。

## 目录与布局

采用集中布局：

```
api/grpc/
  contracts/              # 唯一事实来源（proto）
    buf.yaml
    buf.gen.yaml
    common/v1/*.proto
    powerx/**/v1/*.proto
  gen/                    # Go 生成产物（作为 Go Module 使用）
    go/**
  sdk/                    # 各语言 SDK 包目录（集中发布）
    ts/packages/@powerx/grpc/**
    rust/powerx-sdk/**
    php/powerx-sdk/**
```

## Buf 与生成矩阵

- buf.yaml：开启标准 lint 与文件级 breaking 检查。
- buf.gen.yaml：先保持 Go 生成到 `api/grpc/gen/go`（零改动）；其他语言在各自 SDK 包目录发布时再生成（或由 CI 临时生成）。

## 发布与依赖策略

- Go：以 `api/grpc/gen/go` 为 Go Module；打 `sdk-go/vX.Y.Z` 标签发布；消费者：`go get github.com/ArtisanCloud/PowerX/api/grpc/gen/go@vX.Y.Z`。
- Rust：`api/grpc/sdk/rust/powerx-sdk` 执行 `cargo publish`；消费者：`powerx-sdk = "X.Y.Z"`。
- PHP：`api/grpc/sdk/php/powerx-sdk` 打包发布到 Packagist（建议用 subtree-split 镜像到独立仓），消费者：`composer require artisancloud/powerx-sdk:^X.Y`。
- TS：`api/grpc/sdk/ts/packages/@powerx/grpc` 执行 `npm publish`，消费者：`npm i @powerx/grpc`。

## CI 流水线（标签触发）

- Schema/BSR：`proto/vX.Y.Z` -> `buf lint && buf breaking && buf push`。
- Go：`sdk-go/vX.Y.Z` -> 在 `api/grpc/gen/go` 打 tag 发布（可选 goreleaser）。
- Rust：`sdk-rust/vX.Y.Z` -> `cargo publish`（路径：`api/grpc/sdk/rust/powerx-sdk`）。
- PHP：`sdk-php/vX.Y.Z` -> 构建后通过 Packagist webhook 索引（或镜像仓库自动）。
- TS：`sdk-ts/vX.Y.Z` -> `npm publish`（路径：`api/grpc/sdk/ts/packages/@powerx/grpc`）。

## 版本与兼容

- SemVer：破坏性变更 => major；新增向后兼容 => minor；修复 => patch。
- 破坏性变更时：proto 包名升级到 `v2`（如 `powerx.iam.v2`）；SDK 同步发 `v2.0.0`；与 `v1` 并存一段时间。

## 反射定位

- 仅用于开发调试；生产客户端请使用静态 SDK。

## 过渡策略

- 可将 STS Exchange 暴露为 HTTP 以方便脚本/Rust/PHP 获取 token；业务 RPC 仍走 gRPC。
- SDK 内封装 Token 管理与拦截器，实现“获取 token + 自动注入”的一致体验。

