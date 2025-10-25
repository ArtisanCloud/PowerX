# PowerX Go gRPC SDK

本目录作为 Go Module 使用（临时方案），模块名：

- `module github.com/ArtisanCloud/PowerX/api/grpc/gen/go`

导入示例：

```go
import (
    iamv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

conn, _ := grpc.Dial("127.0.0.1:9001", grpc.WithTransportCredentials(insecure.NewCredentials()))
cli := iamv1.NewMemberServiceClient(conn)
// ...
```

拉取方式：

```bash
go get github.com/ArtisanCloud/PowerX/api/grpc/gen/go@vX.Y.Z
```

生成与更新：在 `api/grpc/contracts/` 运行 `buf generate`。

> 说明：后续如需迁移到 `api/grpc/sdk/go/...` 或独立仓库，将统一调整 `option go_package` 并发布迁移指南。
