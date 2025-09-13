# Rust 集成快速开始（插件侧）

推荐方式：使用官方 `powerx-sdk` crate（封装了 tonic 客户端、STS、TokenManager、拦截器）。在 SDK 发布前，可采用本地生成（tonic-build）作为过渡。

## 方式 A：官方 SDK（推荐）

Cargo.toml：

```toml
[dependencies]
powerx-sdk = "X.Y.Z" # crates.io
tokio = { version = "1", features = ["rt-multi-thread", "macros"] }
```

示例代码：

```rust
use powerx_sdk::{iam::member_client::MemberServiceClient, sts::TokenManager};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let tm = TokenManager::new_from_env()?; // 读取网关地址与凭证
    let channel = tm.channel().await?;      // 自动注入 Bearer
    let mut client = MemberServiceClient::new(channel);

    let resp = client.get_member(/* ... */).await?;
    println!("{:?}", resp);
    Ok(())
}
```

## 方式 B：本地生成（过渡）

Cargo.toml：

```toml
[dependencies]
tonic = "0.11"
prost = "0.12"
tokio = { version = "1", features = ["rt-multi-thread", "macros"] }

[build-dependencies]
tonic-build = "0.11"
```

build.rs：

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure()
        .compile(&[
            "proto/powerx/iam/v1/member.proto",
            // ... 其它需要的 proto
        ], &["proto"])?;
    Ok(())
}
```

项目结构：

```
proto/                # 指向统一 proto 源（git submodule / vendored / BSR 拉取）
  powerx/**/v1/*.proto
build.rs
src/main.rs
```

> 注意：不要复制修改 proto；应从统一仓库或 BSR 同步，避免分叉。

