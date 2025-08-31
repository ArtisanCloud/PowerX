
```bash
# 在 api/grpc/contracts/ 目录执行：

buf lint      # 现在应当 0 警告 0 错误
buf generate  # 生成代码到 ../gen/go/...

api/grpc/gen/go/
├─ powerx/common/v1/...
└─ powerx/organization/v1/...

```
