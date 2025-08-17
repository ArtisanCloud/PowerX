# PowerX 项目目录结构规范

## 项目架构说明

基于领域驱动设计(DDD)的企业级 Go 项目架构，支持多平台、插件化、Pro/Basic 版本差异化。

---

## 一、核心设计原则（总结）

1. **模块化 + 领域化**：按业务域组织 model/用例/仓库，避免“god package”。
2. **分层清晰**：Transport（API/Handler）→ 用例（UseCase）→ 领域模型（Domain Model）+ 抽象仓库 → 持久化实现（GORM）。
3. **可扩展性**：Pro 功能以 extension/plugin 形式，通过 license+feature flag 动态激活。
4. **多平台适配**：不同 client 用 adapter/transformer 做脱敏、字段裁剪、规则变体。
5. **契约复用**：gRPC + grpc-gateway + OpenAPI 共享 contract，HTTP 接口可复用 UseCase。
6. **单一代码库**：基础/Pro 同源，物理目录表达差异，运行时控制权限。

---

## 二、目录结构

```text
/cmd/                               # 应用入口层
  app/
    main.go                        # 加载 bootstrap 并启动应用

/config/                            # 配置文件层
  corex.yaml                        # 基础配置
  corex_pro.yaml                    # Pro 特性开关（license）

/domain/                            # 领域层（纯业务抽象）
  /<domain>/
    /model/                         # 领域模型（纯业务对象，无 ORM tag）
      <entity>.go                   # 实体定义
    /repository/                    # 仓储接口（契约，不含实现）
      <entity>_repository.go
    /service/                       # 可选：领域服务/规则，独立于 infra
      validator.go

/internal/                          # 内部业务逻辑（对外不可见）
  /bootstrap/                       # 应用启动初始化
    bootstrap.go

  /contract/                        # OpenAPI / Proto 契约定义
    /v1/
      <domain>.yaml                # REST/OpenAPI 定义

  /proto/                           # gRPC 协议定义
    /v1/
      <domain>.proto               # gRPC 服务接口
      common.proto                  # 公共类型

  /app/                             # 用例层（业务编排，依赖 domain/repository）
    /<domain>/
      <usecase>.go                  # 如 Create/Get/Submit 等业务用例
    /shared/
      cross_domain_usecase.go       # 跨领域用例

  /infra/                           # 基础设施层（技术实现）
    /persistence/                   # 数据持久化实现
      /<domain>/
        <entity>_repo_gorm.go       # GORM 仓储实现
        <entity>_model.go           # GORM 实体映射
    /database/                      # 数据库工具
      connection.go                 # 连接初始化
      migration.go                  # GORM AutoMigrate 管理
      transaction.go                # 事务封装
    /auth/                          # 鉴权/认证
      jwt.go                        # JWT 中间件
      apikey.go                     # API Key 中间件
    /feature/                       # 系统特性控制
      feature_flag.go               # Feature Flag
      license_checker.go            # License 校验
    /plugin/                        # 插件扩展体系
      loader.go                     # 插件加载
      registry.go                   # 插件注册
      extension_point.go            # 扩展点定义
      /examples/
        sample_plugin/
          hook.go                   # 示例钩子
          plugin_config.yaml        # 插件配置
    /external/                      # 第三方 SDK 适配器
      <sdk_adapter>.go              # 客户端封装

/extensions/                        # Pro 高级功能隔离
  <extension_a>/                    # Pro 插件模块
    register.go                     # 扩展注册入口
  <extension_b>/

/agent/                             # Agent 模块（模板与对接代码）
  /tools/                           # Agent 辅助工具
    registry.go                     # Agent 注册中心
  /templates/                       # Agent 代码模板
    <domain>_agent.go               # 模板示例

/api/                               # 接口层（对外服务）
  /http/                            # HTTP 接口实现
    router.go                       # 路由注册
    /admin/
      <domain>_handler.go          # 管理后台
    /web/
      <domain>_handler.go          # Web 前端
    /miniapp/
      <domain>_handler.go          # 小程序
    /openapi/
      <domain>_handler.go          # OpenAPI
  /grpc/                            # gRPC 服务实现
    /v1/
      <domain>_service.go          # gRPC 服务
    /interceptors/
      auth_interceptor.go           # gRPC 鉴权
      license_interceptor.go        # 许可拦截
      error_mapper.go               # 错误映射
  /dto/                             # DTO：数据传输对象
    /<domain>/
      request.go                    # 请求 DTO
      response.go                   # 响应 DTO
  /adapter/                         # Adapter：DTO ↔ Domain 映射
    /<domain>/
      mapper.go                     # 转换函数

/shared/                            # 跨层共享模块（工具/类型）
  response.go                       # API 统一响应
  errors.go                         # 错误定义
  logger.go                         # 日志封装
  paginator.go                      # 分页工具
  validator.go                      # 通用校验
  mask.go                           # 数据脱敏
  context_key.go                   # Context Key 定义

/integration/                       # 跨模块复合适配器
  <integration_module>.go           # 组合接口实现

/middleware/                        # 中间件
  telemetry.go                      # 链路追踪
  cors.go                           # CORS 策略

/tools/                             # 开发脚本
  gen_openapi.sh                    # 生成 OpenAPI
  gen_proto.sh                      # 生成 gRPC 代码

/deploy/                            # 部署配置
  docker-compose.yaml               # 本地联调
  /k8s/
    *.yaml                          # Kubernetes 配置
  /helm/
    /chart/
      values.yaml                   # Helm 参数

/ci/                                # CI/CD
  pipeline.yaml                     # 流水线定义

/scripts/                           # 脚本工具
  deploy_all.sh                     # 一键部署脚本

/docs/                              # 文档
  architecture.md                   # 架构说明
  api.md                            # API 设计
  openapi.yaml                      # OpenAPI 文档
  grpc_gateway.md                   # gRPC-Gateway 说明
  pro_feature_matrix.md             # Pro 功能矩阵

/test/                              # 测试 & Mock
  /<domain>/
    <usecase>_test.go               # 用例测试
  /mock/
    <domain>/
      mock_<repository>.go          # 仓库 Mock
```

---

## 三、注册账号场景—各层边界举例（以 organization.user 注册为例）

1. **客户端（Admin/Web/MiniApp/OpenAPI/gRPC）**
   发送注册请求（JSON/Proto），如：`{"name","email","password"}`

2. **Adapter（Handler/Service）**

   - 绑定请求体到 DTO：`CreateUserRequest`
   - 验证格式/必填
   - 解析 platform & license（中间件注入）
   - 调用 UseCase：`RegisterUserParams{...}`

3. **UseCase（internal/app/<domain>/user_usecase.go）**

   - `repo.FindByEmail`
   - 密码哈希
   - 构造 `model.NewUser`
   - License/Feature 判断
   - `repo.Save`
   - `plugin.Registry().OnUserRegistered`

4. **Repository 接口**（domain/<domain>/repository）

   ```go
   type UserRepository interface {
     Save(ctx, *User) error
     FindByEmail(ctx, string) (*User, error)
   }
   ```

5. **GORM 实现**（infra/persistence/<domain>/user_repo_gorm.go）

   - `user_model.go` + `repo_gorm.go`
   - AutoMigrate + 唯一索引检查

6. **Transformer**（api/adapter/<domain>/mapper.go）

   - 按平台脱敏/裁剪

7. **Handler**

   - 调用 adapter + UseCase，统一返回

---

## 四、Pro 增值融合

- `/extensions/` 物理隔离 Pro 功能
- 动态路由加载：`if license.IsPro ...`
- Feature Flag 控制能力
- 插件扩展通过 `infra/plugin`

---

## 五、多平台适配与脱敏

- `api/adapter/<domain>/mapper.go`
- `shared/mask.go` 配置化脱敏
- Pro 低码驱动配置

---

## 六、数据库迁移

- `infra/persistence/*_model.go`
- `infra/database/migration.go` AutoMigrate
- 启动时执行

---

## 七、License & Flag

- `infra/feature/*`
- `feature.Enabled` + `license_checker`

---

## 八、架构原则

- 分层清晰
- 依赖倒置
- 多平台支持
- 插件化扩展
- 版本差异化
- 配置驱动

---

## 九、实施建议

1. 分阶段实施
2. 依赖注入（wire/fx）
3. 测试覆盖
4. 文档同步
