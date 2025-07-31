# PowerX 项目

基于领域驱动设计(DDD)的企业级Go项目架构，支持多平台、插件化、Pro/Basic版本差异化。

## 项目结构

```
PowerX/
├── cmd/app/                    # 应用启动入口
├── config/                     # 配置文件
├── internal/                   # 内部代码
│   ├── contract/               # 外部契约定义
│   ├── proto/                  # gRPC协议定义
│   ├── domain/                 # 领域模型层
│   │   ├── organization/       # 组织领域
│   │   └── e_commerce/product/ # 电商产品领域
│   ├── app/                    # 用例层
│   ├── infra/                  # 基础设施层
│   │   ├── persistence/        # 数据持久化
│   │   ├── database/           # 数据库管理
│   │   ├── auth/               # 认证授权
│   │   ├── feature/            # 特性开关
│   │   ├── plugin/             # 插件系统
│   │   └── external/           # 外部服务
│   ├── extensions/             # Pro版本扩展
│   ├── api/                    # API适配层
│   │   ├── http/               # HTTP接口
│   │   └── grpc/               # gRPC接口
│   ├── dto/                    # 数据传输对象
│   ├── adapter/                # 适配器层
│   ├── shared/                 # 共享工具
│   ├── integration/            # 集成适配器
│   └── middleware/             # 中间件
├── deploy/                     # 部署配置
├── ci/                         # CI/CD配置
├── scripts/                    # 脚本文件
└── docs/                       # 文档
```

## 架构原则

1. **分层清晰**：Domain -> Application -> Infrastructure -> API
2. **依赖倒置**：高层不依赖低层，都依赖抽象
3. **多平台支持**：Admin/Web/MiniApp/OpenAPI 差异化处理
4. **插件化扩展**：支持动态加载和扩展点
5. **版本差异化**：Pro/Basic 特性物理分离
6. **配置驱动**：环境配置覆盖机制

## 快速开始

1. 启动应用：
```bash
go run cmd/app/main.go
```

2. 访问API：
- HTTP: http://localhost:8080
- gRPC: localhost:9090

## 开发指南

详细的架构说明请参考：`.codebuddy/.rules/project-directory.mdc`