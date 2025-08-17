# 实体注册流程蓝图

## 概述

本文档定义了完整的实体注册业务流程，包含数据验证、密码加密、数据持久化和审计日志。该流程遵循DDD（领域驱动设计）架构原则，采用四层架构模式。支持自定义领域（domain）和实体（entity），可适用于用户注册、组织用户注册、管理员注册等多种场景。

## 流程变量

- **项目名称**: `{{.project_name}}`
- **领域**: `{{.domain}}` (可配置，如: `user`, `organization`, `organization/user`)
- **实体**: `{{.entity}}` (可配置，如: `User`, `OrganizationUser`)
- **数据表**: `{{.table_name}}` (可配置，如: `users`, `organization_users`)
- **唯一字段**: `{{.unique_fields}}` (可配置，如: `["email"]`, `["email", "organization_id"]`)
- **密码字段**: `{{.password_field}}` (可配置，默认: `password`)
- **审计启用**: `{{.audit_enabled}}` (可配置，默认: `true`)

## 执行步骤

### 1. 幂等性保护

**目标**: 防止重复提交，基于请求ID或用户标识进行幂等性检查

**参数**:
- 关键字段: `["email", "phone"]`
- 缓存时长: `300秒`

**超时**: 5秒  
**重试**: 1次

### 2. 唯一性验证

**目标**: 验证邮箱、手机号等关键字段的唯一性

**参数**:
- 验证字段: `{{.unique_fields}}`
- 数据表: `{{.table_name}}`
- 错误信息: `{{.entity}}已存在`

**前置条件**: 幂等性保护通过  
**超时**: 10秒  
**重试**: 2次

### 3. 输入数据验证

**目标**: 验证用户输入数据的格式和完整性

**验证规则**:
- **邮箱**: 必填，邮箱格式
- **密码**: 必填，最少8位，包含大小写字母和数字
- **姓名**: 必填，2-50字符
- **手机**: 可选，11位数字格式

**前置条件**: 唯一性验证通过  
**超时**: 5秒  
**重试**: 1次

### 4. 密码加密

**目标**: 使用bcrypt算法对用户密码进行安全加密

**参数**:
- 算法: `bcrypt`
- 成本因子: `12`
- 目标字段: `{{.password_field}}`

**前置条件**: 输入验证通过  
**超时**: 10秒  
**重试**: 1次

## 代码生成步骤

### 5. 生成领域模型

**目标**: 生成{{.entity}}领域模型结构体

**输出路径**: `domain/{{.domain}}/model/{{.entity | lower}}.go`

**字段定义**:
- `id`: uint, 主键自增
- `email`: string, 唯一索引，非空，255字符
- `password`: string, 非空，255字符
- `name`: string, 非空，100字符
- `phone`: string, 20字符
- `status`: int, 默认值1
- `created_at`: time.Time, 自动创建时间
- `updated_at`: time.Time, 自动更新时间

### 6. 生成GORM模型

**目标**: 生成GORM数据库模型

**输出路径**: `internal/infra/persistence/{{.domain}}/model/{{.entity | lower}}_model.go`

### 7. 生成仓库接口

**目标**: 生成{{.entity}}仓库接口定义

**输出路径**: `domain/{{.domain}}/repository/{{.entity | lower}}_repository.go`

### 8. 生成仓库实现

**目标**: 生成{{.entity}}仓库GORM实现

**输出路径**: `internal/infra/persistence/{{.domain}}/repository/{{.entity | lower}}_repo_gorm.go`

**特性**: 继承BaseRepository，提供基础CRUD操作

### 9. 生成数据库迁移

**目标**: 生成数据库迁移文件

**输出路径**: `internal/infra/database/migration.go`

### 10. 生成业务用例

**目标**: 生成{{.entity}}注册业务逻辑层代码

**输出路径**: `internal/app/{{.domain}}/{{.entity | lower}}.go`

**方法**:
- `Register(ctx context.Context, req *RegisterRequest) (*{{.entity}}, error)`: {{.entity}}注册业务逻辑
- `CheckEmailExists(ctx context.Context, email string) (bool, error)`: 检查邮箱是否已存在
- `ValidatePassword(password string) error`: 验证密码强度

### 11. 生成DTO

**目标**: 生成数据传输对象

**输出路径**: `api/dto/{{.domain}}/{{.entity | lower}}_dto.go`

**DTO类型**:
- `RegisterRequest`: 注册请求（email, password, name, phone）
- `RegisterResponse`: 注册响应（id, email, name, status, created_at）
- `CheckEmailResponse`: 邮箱检查响应（exists）

### 12. 生成适配器

**目标**: 生成领域模型与DTO之间的适配器

**输出路径**: `api/adapter/{{.domain}}/{{.entity | lower}}_adapter.go`

### 13. 生成API路由

**目标**: 生成{{.entity}}注册REST API路由

**输出路径**: `api/http/router.go`

**端点**:
- `POST /api/v1/{{.domain | replace "/" "-"}}/register`: {{.entity}}注册
- `GET /api/v1/{{.domain | replace "/" "-"}}/check-email`: 邮箱检查

### 14. 生成HTTP处理器

**目标**: 生成HTTP请求处理器

**输出路径**: `api/http/admin/{{.domain}}/{{.entity | lower}}_handler.go`

**处理器**:
- `RegisterHandler`: POST /register
- `CheckEmailHandler`: GET /check-email

### 15. 生成gRPC服务

**目标**: 生成gRPC服务定义

**输出路径**: `api/grpc/v1/{{.domain}}/{{.domain}}_service.go`

**服务方法**:
- `Register(RegisterRequest) RegisterResponse`
- `CheckEmail(CheckEmailRequest) CheckEmailResponse`

### 16. 审计日志

**目标**: 设置{{.entity}}注册操作的审计日志记录

**事件类型**:
- `{{.domain | replace "/" "_"}}_registration_attempt`: 注册尝试（info级别）
- `{{.domain | replace "/" "_"}}_registration_success`: 注册成功（info级别）
- `{{.domain | replace "/" "_"}}_registration_failed`: 注册失败（warn级别）
- `duplicate_{{.domain | replace "/" "_"}}_registration_attempt`: 重复注册尝试（warn级别）

### 17. 生成测试代码

**目标**: 生成单元测试和集成测试代码

**输出路径**: `test/{{.domain}}/{{.entity | lower}}_test.go`

**测试类型**:
- **单元测试**: UseCase层（Register, CheckEmailExists, ValidatePassword）
- **集成测试**: API层（RegisterHandler, CheckEmailHandler）
- **仓库测试**: Repository层（Create, FindByEmail, ExistsByEmail）

### 18. 生成Mock对象

**目标**: 生成测试用Mock对象

**输出路径**: `test/mock/{{.domain}}/{{.entity | lower}}_mock.go`

**Mock接口**:
- `{{.entity}}Repository`
- `{{.entity}}UseCase`

## 流程配置

- **最大并行步骤**: 2
- **超时策略**: 快速失败
- **重试策略**: 指数退避
- **回滚启用**: true

## 输出结构

### 领域层
- 领域模型: `domain/{{.domain}}/model/{{.entity | lower}}.go`
- 仓库接口: `domain/{{.domain}}/repository/{{.entity | lower}}_repository.go`

### 基础设施层
- GORM模型: `internal/infra/persistence/{{.domain}}/model/{{.entity | lower}}_model.go`
- 仓库实现: `internal/infra/persistence/{{.domain}}/repository/{{.entity | lower}}_repo_gorm.go`
- 数据库迁移: `internal/infra/database/migration.go`

### 应用层
- 业务用例: `internal/app/{{.domain}}/{{.entity | lower}}.go`

### 接口层
- DTO: `api/dto/{{.domain}}/{{.entity | lower}}_dto.go`
- 适配器: `api/adapter/{{.domain}}/{{.entity | lower}}_adapter.go`
- 路由: `api/http/router.go`
- 处理器: `api/http/admin/{{.domain}}/{{.entity | lower}}_handler.go`
- gRPC服务: `api/grpc/v1/{{.domain}}/{{.domain}}_service.go`

### 测试层
- 测试代码: `test/{{.domain}}/{{.entity | lower}}_test.go`
- Mock对象: `test/mock/{{.domain}}/{{.entity | lower}}_mock.go`

## 架构原则

1. **领域驱动设计**: 以业务领域为核心组织代码
2. **分层架构**: 清晰的四层架构分离关注点
3. **依赖倒置**: 高层模块不依赖低层模块
4. **接口隔离**: 每个接口职责单一
5. **开闭原则**: 对扩展开放，对修改封闭

## 配置示例

### 用户注册场景
```yaml
domain: "user"
entity: "User"
table_name: "users"
unique_fields: ["email"]
password_field: "password"
audit_enabled: true
```

### 组织用户注册场景
```yaml
domain: "organization/user"
entity: "OrganizationUser"
table_name: "organization_users"
unique_fields: ["email", "organization_id"]
password_field: "password"
audit_enabled: true
```

### 管理员注册场景
```yaml
domain: "admin"
entity: "Admin"
table_name: "admins"
unique_fields: ["username", "email"]
password_field: "password_hash"
audit_enabled: true
```

## 使用说明

1. 根据具体业务需求调整变量配置
2. 支持嵌套domain路径（如: `organization/user`）
3. 按照步骤顺序执行代码生成
4. 确保每个步骤的前置条件满足
5. 关注超时和重试配置
6. 验证生成的代码符合项目规范

---

*本文档基于CoreX框架的DDD架构规范，遵循最佳实践和设计模式。*