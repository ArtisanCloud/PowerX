# Capability Generator

用于将 OpenAPI / gRPC proto 自动生成 PowerX 平台能力配置（`platform_capabilities` YAML）。

## 快速开始

在 `backend` 目录执行：

```bash
./scripts/capability_registry/generate_from_specs.sh \
  --openapi api/openapi/swagger.json \
  --proto api/grpc/contracts \
  --out config/platform_capabilities/generated.auto.yaml
```

仅预览（不落盘）：

```bash
./scripts/capability_registry/generate_from_specs.sh \
  --openapi api/openapi/swagger.json \
  --proto api/grpc/contracts/powerx/workflow/v1/workflow.proto \
  --dry-run
```

## 说明

- 生成器入口：`cmd/capability_gen`
- 默认输出：`backend/config/platform_capabilities/generated.auto.yaml`
- 该工具为自动生成 MVP：
  - OpenAPI：按 path + method 生成 REST capabilities
  - Proto：按 service + rpc 生成 gRPC capabilities
- 建议把生成文件作为 `generated` 来源，再由人工复核后合并到正式能力配置。
