
```bash
# 在 api/grpc/contracts/ 目录执行：

buf lint      # 现在应当 0 警告 0 错误
buf generate  # 生成代码到 ../gen/go/...

api/grpc/gen/go/
├─ powerx/common/v1/...
└─ powerx/organization/v1/...

```

## Buf 配置说明

- Event Fabric 契约已统一迁移至 `powerx/event_fabric/v1`，`buf.gen.yaml` 默认前缀即可生成到 `github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/event_fabric/v1`。
- `buf.yaml` 使用 `ignore_only` 精准豁免 `powerx/capability/registry/v1/registry.proto` 的 `RPC_REQUEST_STANDARD_NAME` 规则；新契约必须遵守默认 lint 规范，避免增加忽略范围。
