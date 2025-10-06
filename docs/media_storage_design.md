# 媒体存储多驱动设计方案（DDD 适配版）

## 设计目标
- 在 DDD 架构下抽象统一的媒体资源上下文，屏蔽不同底层存储的差异。
- 对齐现有 `api → handler → service → repository → model` 分层约定，保证调用路径清晰。
- 复用 `internal/infra` 目录下的基础设施布局，引入 `MediaManager` 作为驱动中心。
- 支持本地文件系统（LocalStorage）与兼容 S3 协议的对象存储（MinIO/OSS/S3）。

## 分层架构与目录规划
结合当前项目的 DDD 实践（`domain` 统一实体、`internal/service` 承载用例、`internal/transport/http/admin` 存放 Handler），媒体模块作为主服务内部能力继续随业务模块部署，不拆分独立进程，其推荐目录如下：

```
pkg/corex/db/persistence/
  model/
    media/
      media_asset.go      # ORM/DAO 模型，复用全局 persistence 规范
  repository/
    media/
      media_repo.go       # 媒体仓储实现，直接供 Service 注入

internal/infra/
  media/
    manager/
      manager.go          # MediaManager，负责驱动注册、选择
    driver/
      driver.go           # StorageDriver 接口定义
      local/
        driver.go
      s3/
        driver.go         # 基于 go-minio，兼容多平台 S3
    metadata/
      repository.go       # （可选）直接访问底层 DB/缓存的实现

internal/service/media/
  media_service.go        # 封装业务用例，依赖 MediaManager 和领域仓储

internal/transport/http/admin/media/
  handler.go              # Admin API Handler，组装请求/响应
  routes.go               # 注册 `/admin/media` 路由

config/
  storage.go              # 新增存储驱动配置体、默认值加载
```

> 说明：`internal/infra/media` 与现有 `internal/infra/plugin` 平级，保持基础设施职责集中。`MediaManager` 的职责与 `PluginManager` 类似：对外提供统一接口，对内协调驱动与配置。

### 模型与仓储实现规范

- **ORM 模型**：`pkg/corex/db/persistence/model/media/media_asset.go` 下的结构体需嵌入 `model.PowerUUIDModel`（或 `model.PowerModel`，视主键策略而定），以继承统一的 `ID/UUID`、时间戳、软删除字段；字段命名、GORM Tag 与 JSON Tag 应对齐同级模块（如 `model/iam`）。
- **仓储实现**：`pkg/corex/db/persistence/repository/media/media_repo.go` 中的仓储结构体须内嵌 `*repository.BaseRepository[dbm.MediaAsset]`，并通过 `repository.NewBaseRepository` 构造基础 CRUD 能力，同时根据媒体场景扩展自定义查询方法。调用方式可参考 `tenant_repo.go`，直接在 Service 中注入仓储结构体而非额外定义领域接口。

## MediaManager 职责与协作
```
HTTP Handler → MediaService → MediaRepo + MediaManager
                                  |                |
                                  v                v
                 domain/media + pkg/corex/db/persistence    internal/infra/media
```

- **MediaManager**：
  - 依据配置初始化各类驱动，维护驱动映射、默认驱动等状态。
  - 暴露 `Put`, `Get`, `Delete`, `GenerateURL`, `List` 等统一接口。
  - 管理驱动生命周期（初始化、健康检查、关闭）。
- **StorageDriver 接口**：定义驱动的最小集合方法；Local/S3 驱动分别实现。
- **MediaService（用例层）**：
  - 处理租户/用户权限、命名空间、业务流程（缩略图、回源、策略）。
  - 调用 `pkg/corex/db/persistence/repository/media.MediaRepository`（结构体）读写媒体元数据，同时利用 `MediaManager` 完成实际文件操作。

## 配置结构
结合现有 `config` 模块和 YAML/ENV 约定，新增 `Storage` 配置体：

```yaml
storage:
  enabled: true
  defaultDriver: "s3"
  drivers:
    local:
      root: "./storage"
      baseURL: "http://localhost:8080/static"
      cleanOnInit: false
    s3:
      endpoint: "http://minio:9000"
      region: "us-east-1"
      bucket: "powerx-media"
      accessKey: ${MINIO_ACCESS_KEY}
      secretKey: ${MINIO_SECRET_KEY}
      useSSL: false
      pathStyle: true
      presignExpiration: 3600
```

- `config/storage.go` 定义结构体、默认值与校验逻辑，并在 `config.LoadConfig` 中绑定。
- `internal/bootstrap` 在应用启动时构造 `MediaManager`，注入至 `internal/app/shared.Deps`，供 Handler/Service 使用。

## 核心业务流程（结合层级）
1. **上传**
   - `internal/transport/http/admin/media/handler.go` 接收请求，解析租户/用户信息。
   - 调用 `internal/service/media.MediaService.Upload`，完成权限校验、元数据构造。
   - `MediaService` 先调用 `MediaManager.Put` 将文件写入实际驱动，再通过媒体仓储结构体将元数据落库。

2. **访问/下载**
   - Handler 调用 `MediaService.Get`。
   - `MediaService` 查询元数据、判断驱动类型：
     - Local：返回 `baseURL + relativePath` 或走内部代理。
     - S3：通过 `MediaManager.GenerateURL` 生成临时签名地址。

3. **删除/回收**
   - `MediaService.Delete` 同时更新元数据状态、调用驱动删除。
   - 支持后台任务扫描过期文件（可挂载在 `internal/server/agent` 定时任务或 Cron）。

## MinIO / S3 驱动实现要点
- 使用 `github.com/minio/minio-go/v7` 作为统一客户端，兼容 AWS S3、阿里云 OSS、华为 OBS 等。
- 启动时由 `MediaManager` 检测桶是否存在，不存在则按配置创建（可配置开关）。
- 支持 `PutObject`, `GetObject`, `RemoveObject`, `PresignedGetObject` 等常用操作，封装重试与错误处理。
- 驱动层仅关注对象操作，业务逻辑在 `MediaService`；符合 DDD 的基础设施定义。

## 与现有组件的协同
- **PluginManager**：两者均位于 `internal/infra`，可以共享日志、配置加载、健康检查模式，保持基础设施层的一致性。
- **shared.Deps**：在 `internal/bootstrap` 构造 `Deps` 时，新增 `MediaManager`、`MediaService` 注入，便于 Handler 直接获取。
- **Audit & Setting**：如需结合系统设置，可通过 `internal/service/system` 提供的 `SettingService` 获取上传限制、生命周期策略。

## 测试策略
- **单元测试**：
  - `internal/infra/media/driver/local` 使用临时目录测试读写。
  - `internal/infra/media/driver/s3` 配合 Docker Compose 启动 MinIO，验证上传/删除/签名。
- **集成测试**：
  - 针对 `internal/transport/http/admin/media` 的路由，编写 API 测试用例（mock JWT/租户）。
- **性能测试**：
  - 通过压测工具模拟大文件上传、并发访问，优化连接池、分块上传策略。

## 迭代步骤
1. 定义 `domain/media/entity.go`（如需额外领域对象）以及 `pkg/corex/db/persistence/model/media/media_asset.go`，并补充迁移脚本（如 `media_assets` 表）。
2. 实现 `internal/infra/media` 模块（Manager + Drivers），并在 `bootstrap` 注册。
3. 编写 `internal/service/media` 用例层逻辑及仓储实现。
4. 实现 `internal/transport/http/admin/media` 的 Handler、路由、请求/响应结构体。
5. 补充配置、测试、文档与运维（MinIO 部署、监控、备份）。

通过以上规划，媒体存储模块能够在 PowerX 的 DDD 架构中有机融入，实现 `api → handler → service → repository → model` 的清晰调用路径，同时借助 `internal/infra/media` 的 MediaManager 统一管理多种存储驱动。这样既满足本地开发需求，也可以借助 MinIO 兼容多家云端 S3 协议对象存储。
