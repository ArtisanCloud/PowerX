
```bash
# 在 api/grpc/contracts/ 目录执行：

buf lint      # 现在应当 0 警告 0 错误
buf generate  # 生成代码到 ../gen/go/...

api/grpc/gen/go/
├─ powerx/common/v1/...
└─ powerx/organization/v1/...

```

## Buf 配置说明

- `buf.gen.yaml` 通过 `managed.go_package_prefix.overrides` 为 `corex/event_fabric/v1` 指定生成路径 `github.com/ArtisanCloud/PowerX/api/grpc/gen/go/corex/event_fabric/v1`，保持与目录和 import 规范一致。
- `buf.yaml` 使用 `ignore_only` 精准豁免 `powerx/capability/registry/v1/registry.proto` 的 `RPC_REQUEST_STANDARD_NAME` 规则；新契约必须遵守默认 lint 规范，避免增加忽略范围。
