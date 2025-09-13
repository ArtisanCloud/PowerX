# PHP 集成快速开始（插件侧）

推荐方式：使用官方 `artisancloud/powerx-sdk` Composer 包（包含 gRPC stub 与 STS/Token 管理）。在 SDK 发布前，可采用本地生成作为过渡。

## 方式 A：官方 SDK（推荐）

composer.json：

```json
{
  "require": {
    "artisancloud/powerx-sdk": "^X.Y",
    "grpc/grpc": "^1.6",
    "google/protobuf": "^3.25"
  }
}
```

示例代码：

```php
<?php
use PowerX\Iam\V1\MemberServiceClient;

$addr = getenv('POWERX_GRPC_GATEWAY') ?: '127.0.0.1:9001';
$client = new MemberServiceClient($addr, [
    'credentials' => Grpc\ChannelCredentials::createInsecure(),
]);

// TODO: 通过 SDK 的 TokenManager 注入 Bearer；此处省略
list($resp, $status) = $client->GetMember($request)->wait();
```

## 方式 B：本地生成（过渡）

先安装 protoc 与 `grpc_php_plugin`，然后：

```bash
protoc \
  --php_out=gen/php \
  --grpc_out=gen/php \
  --plugin=protoc-gen-grpc=$(which grpc_php_plugin) \
  -I proto proto/powerx/iam/v1/member.proto
```

Composer 依赖运行时：

```bash
composer require google/protobuf:^3.25 grpc/grpc:^1.6
```

> 注意：proto 源请统一引用官方仓库或 BSR；不要复制修改。

