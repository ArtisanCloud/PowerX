# 媒体管理后台 API 设计（对齐 handler → service → repository → model 模式）

## 设计目标
- 为后台上传、管理媒体资源提供统一的 RESTful API，遵循现有 `api → handler → service → repository → model` 分层。
- 复用 `internal/transport/http/admin` 目录下的 Handler 组织方式，配合 DTO 校验、统一响应结构。
- 与 `internal/service/media`、`pkg/corex/db/persistence/repository/media` 的实现保持解耦。

## 目录规划
```
internal/transport/http/admin/media/
  asset_handler.go     # Handler 与路由注册
  dto.go               # 可选：请求/响应 DTO（若字段较多，按模块集中）
  routes.go            # 模块内子路由，供 admin/routes.go 汇总
```

`admin/routes.go` 中新增：
```go
mediaGroup := admin.Group("/media")
media.RegisterRoutes(mediaGroup, deps.MediaHandler)
```

## 路由与职责
| HTTP 方法 | 路径 | Handler 方法 | 用途 | 说明 |
|-----------|------|---------------|------|------|
| POST | `/api/v1/admin/media/assets` | `UploadAsset` | 上传文件并创建媒体记录 | 接收 `multipart/form-data`，调用 `MediaService.Upload` |
| GET | `/api/v1/admin/media/assets` | `ListAssets` | 分页查询媒体资源 | 支持关键字、驱动类型过滤 |
| GET | `/api/v1/admin/media/assets/:id` | `GetAsset` | 查询单条媒体详情 | 返回元数据及可访问地址 |
| PATCH | `/api/v1/admin/media/assets/:id` | `UpdateAsset` | 更新名称、标签、业务状态 | 仅修改业务字段，避免覆盖存储信息 |
| DELETE | `/api/v1/admin/media/assets/:id` | `DeleteAsset` | 软删除媒体并触发驱动删除 | 调用 `MediaService.Delete` |
| POST | `/api/v1/admin/media/assets/presign` | `GeneratePresignedURL` | 获取直传/下载预签名 URL | 复用 `MediaManager.GenerateURL` |

> 统一使用 `/api/v1/admin` 前缀，由 `cmd/agent`/`internal/server/http` 的现有路由注册逻辑拼接。

## Handler 层设计
```go
// internal/transport/http/admin/media/asset_handler.go
type AssetHandler struct {
    S *service.MediaService
}

func NewAssetHandler(s *service.MediaService) *AssetHandler { return &AssetHandler{S: s} }
```

### 请求校验与响应
- 延续 `dto.ValidateRequestWithContext`、`dto.ResponseSuccess`、`dto.ResponseList`、`dto.ResponseError` 等助手。
- 列表请求示例：
```go
type ListAssetsReq struct {
    dto.PaginationRequest
    Keyword   string  `form:"keyword"`
    Driver    *string `form:"driver"`   // local / s3
    OwnerType *string `form:"ownerType"`
    OwnerID   *uint64 `form:"ownerId"`
}
```
- 更新请求示例：
```go
type UpdateAssetReq struct {
    Name        *string   `json:"name"`
    Tags        *[]string `json:"tags"`
    Description *string   `json:"description"`
    Status      *int8     `json:"status"`
}
```
- 上传接口需要处理 `multipart.FileHeader`，同时兼容直接指定外链（如采用预签名直传回调）：
```go
type UploadAssetReq struct {
    File       *multipart.FileHeader `form:"file"`
    FileURL    *string               `form:"file_url"` // 预签名/外链回填
    Driver     *string               `form:"driver"`
    Folder     *string               `form:"folder"`
    OwnerType  *string               `form:"owner_type"`
    OwnerID    *uint64               `form:"owner_id"`
    Meta       string                `form:"meta"`     // JSON 字符串
}
```

### Handler 调用链
1. Handler 解析并校验请求参数。
2. 调用 `MediaService` 对应方法，并传递 `context.Context` 与请求者身份（可从 `dto` 或 `middleware` 中取得）。
3. 根据返回值使用 `dto.ResponseSuccess/List/Error` 输出统一 JSON。
4. 错误码：遵循现有惯例，业务错误返回 `http.StatusBadRequest` 或 `http.StatusConflict`，权限错误用 `http.StatusForbidden`。

## Service 层接口
`internal/service/media` 暴露：
```go
type MediaService struct {
    Repo *mediarepo.MediaRepository
    Manager *infra.MediaManager
}

type UploadInput struct {
    File       io.Reader
    FileName   string
    Size       int64
    ContentType string
    Driver     string
    Folder     string
    OwnerType  *string
    OwnerID    *uint64
    Meta       map[string]any
    OperatorID uint64
}

func (s *MediaService) Upload(ctx context.Context, in UploadInput) (*dbm.MediaAsset, error)
func (s *MediaService) List(ctx context.Context, opt ListOption) ([]dbm.MediaAsset, int64, error)
func (s *MediaService) Get(ctx context.Context, id uint64) (*dbm.MediaAsset, error)
func (s *MediaService) Update(ctx context.Context, id uint64, in UpdateInput) (*dbm.MediaAsset, error)
func (s *MediaService) Delete(ctx context.Context, id uint64, soft bool) error
func (s *MediaService) GeneratePresignedURL(ctx context.Context, in PresignInput) (*PresignOutput, error)
```

- 其中 `mediarepo` 建议引用 `github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media` 包，通过 `mediarepo.NewMediaRepository(db)`（命名可参考 `NewTenantRepository`）构造实例后注入。
- `Upload` 会先通过 `MediaManager.Put` 写入目标驱动，再调用 `Repo.Create` 保存 `MediaAsset`。
- `List` 组合仓储的分页能力与业务过滤，返回分页总数以配合 Handler 响应。
- `Delete` 除软删除数据库记录外，还需根据配置决定是否物理删除对象。

## 仓储层契约
- 仓储实现位于 `pkg/corex/db/persistence/repository/media/media_repo.go`，结构体需内嵌 `*repository.BaseRepository[dbm.MediaAsset]` 并实现分页筛选逻辑（驱动类型、所有者、标签等），使用方式可参考 `TenantRepository` 直接注入 Service。

## ORM 模型约束
- `pkg/corex/db/persistence/model/media/media_asset.go`：
```go
package media

type MediaAsset struct {
    model.PowerUUIDModel

    Driver      string `gorm:"column:driver;size:32" json:"driver"`
    Bucket      string `gorm:"column:bucket;size:128" json:"bucket"`
    ObjectKey   string `gorm:"column:object_key;size:512" json:"objectKey"`
    FileName    string `gorm:"column:file_name;size:256" json:"fileName"`
    ContentType string `gorm:"column:content_type;size:128" json:"contentType"`
    Size        int64  `gorm:"column:size" json:"size"`
    Folder      string `gorm:"column:folder;size:256" json:"folder"`
    Tags        datatypes.JSON `gorm:"column:tags" json:"tags"`
    OwnerType   *string `gorm:"column:owner_type;size:64" json:"ownerType"`
    OwnerID     *uint64 `gorm:"column:owner_id" json:"ownerId"`
    Meta        datatypes.JSON `gorm:"column:meta" json:"meta"`
    Status      int8 `gorm:"column:status" json:"status"`
}
```
- 嵌入 `model.PowerUUIDModel`，在 `BeforeCreate` 中自动生成 UUID；如需自定义表名，通过 `func (MediaAsset) TableName() string` 返回 `"media_assets"`。

## 响应 DTO 建议
- 列表响应可直接返回 `dbm.MediaAsset`，若需附加临时访问地址，可在 Handler 层生成视图：
```go
type mediaView struct {
    dbm.MediaAsset
    URL string `json:"url"`
}
```
- 对于预签名 URL，定义专用响应：
```go
type PresignResp struct {
    URL        string            `json:"url"`
    ExpireAt   time.Time         `json:"expireAt"`
    Method     string            `json:"method"`
    FormFields map[string]string `json:"formFields,omitempty"`
}
```

## 安全与鉴权
- 复用现有 `admin` 路由鉴权（JWT + 租户上下文），Handler 通过 `dto.GetCurrentUser`（若已有）获取操作者。
- 上传接口需限制文件大小、MIME 类型，可在 Service 层进行白名单校验。
- 支持通过 `OwnerType/OwnerID` 控制多租户隔离；仓储查询默认根据租户过滤。

## 错误处理与日志
- Handler 捕获 Service 返回的业务错误，自定义 `dto.ResponseError` 消息。
- Service 应结合 `github.com/ArtisanCloud/PowerX/pkg/errorx`（若存在）包装业务错误码，便于前端识别。
- `MediaManager` 错误需带上驱动、ObjectKey 等上下文，写入统一日志。

## 后续扩展
- 若未来引入后台任务，可在 `internal/transport/http/admin/media/tasks_handler.go` 下新增任务管理接口。
- 支持批量删除、批量标签更新时，遵循相同分层设计：Handler 解析数组，Service 事务处理，Repo 调用批量接口。
- 若要开放给第三方，可在 `internal/transport/http/api/media` 下新建公共 API，重用同一 Service。
