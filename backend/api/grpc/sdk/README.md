PowerX SDK Monorepo（集中在 api/grpc/sdk 下）

目录约定：
- ts/packages/@powerx/grpc      # TypeScript SDK（npm 包）
- rust/powerx-sdk               # Rust SDK（crates.io）
- php/powerx-sdk                # PHP SDK（Packagist）

说明：
- Go SDK 暂保持在 `api/grpc/gen/go` 作为独立 Go Module（后续可迁移到此处或独立仓库）。
- 生成产物可由 CI 在发布时临时产出，然后打包发布；日常开发不必提交大量生成文件。

