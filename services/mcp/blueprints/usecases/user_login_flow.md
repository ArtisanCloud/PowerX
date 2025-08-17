# 实体登录流程蓝图

## 流程概述

**流程ID**: `user_login_flow`  
**流程名称**: 实体登录流程  
**版本**: 1.0.0  
**描述**: 完整的实体登录业务流程，支持自定义领域和实体，包含身份验证、密码验证、会话管理和安全审计

## 流程变量

| 变量名 | 默认值 | 描述 |
|--------|--------|------|
| `project_name` | `{{.project_name}}` | 项目名称 |
| `domain` | `{{.domain}}` | 领域名称，支持嵌套路径如 organization/user |
| `entity` | `{{.entity}}` | 实体名称 |
| `table_name` | `{{.table_name}}` | 实体表名 |
| `session_table` | `{{.entity | lower}}_sessions` | 会话表名 |
| `login_attempts_table` | `login_attempts` | 登录尝试表名 |
| `jwt_secret` | `{{.jwt_secret}}` | JWT密钥 |
| `session_duration` | `86400` | 会话持续时间（秒） |
| `max_login_attempts` | `5` | 最大登录尝试次数 |
| `lockout_duration` | `900` | 账户锁定时长（秒） |
| `audit_enabled` | `true` | 是否启用审计日志 |

## 执行步骤

### 1. 登录频率限制 (rate_limit_check)
- **类型**: 验证
- **描述**: 检查登录请求频率，防止暴力破解攻击
- **参数**:
  - 最大尝试次数: 5次
  - 时间窗口: 300秒
  - 识别字段: IP地址、邮箱
  - 锁定时长: 900秒
- **超时**: 5秒
- **重试**: 1次

### 2. 输入数据验证 (input_validation)
- **类型**: 验证
- **描述**: 验证登录请求数据的格式和完整性
- **验证规则**:
  - `email`: 必填，邮箱格式
  - `password`: 必填，字符串类型
  - `remember_me`: 可选，布尔类型
  - `device_info`: 可选，设备信息对象
- **前置条件**: 频率限制检查通过
- **超时**: 5秒

### 3. {{.entity}}查找 ({{.entity | lower}}_lookup)
- **类型**: 查询
- **描述**: 根据邮箱查找{{.entity}}记录
- **查询字段**: email
- **返回字段**: id, email, password, status, failed_login_attempts, locked_until
- **前置条件**: 输入验证通过
- **超时**: 10秒
- **重试**: 2次

### 4. 账户状态检查 (account_status_check)
- **类型**: 验证
- **描述**: 检查{{.entity}}账户状态和锁定情况
- **检查项**:
  - 账户是否激活
  - 是否被锁定
  - 失败尝试次数
- **前置条件**: {{.entity}}查找成功
- **超时**: 5秒

### 5. 密码验证 (password_verification)
- **类型**: 安全
- **描述**: 验证{{.entity}}输入的密码是否正确
- **加密算法**: bcrypt
- **前置条件**: 账户状态正常
- **超时**: 10秒

### 6. 登录尝试处理 (login_attempt_handler)
- **类型**: 条件处理
- **描述**: 根据密码验证结果处理登录尝试
- **成功操作**:
  - 重置失败尝试次数
  - 更新最后登录时间
- **失败操作**:
  - 增加失败尝试次数
  - 检查锁定阈值
  - 记录失败尝试
- **超时**: 15秒

### 7. 会话生成 (session_generator)
- **类型**: 安全
- **描述**: 为成功登录的{{.entity}}生成会话令牌
- **令牌类型**: JWT
- **包含声明**: user_id, email, role, permissions
- **会话时长**: 86400秒（24小时）
- **记住我时长**: 2592000秒（30天）
- **前置条件**: 登录成功
- **超时**: 10秒

## 代码生成步骤

### 8. 生成领域模型 (domain_model_generator)
- **描述**: 生成{{.domain}}相关的领域模型
- **生成文件**:
  - `domain/{{.domain}}/model/{{.entity | lower}}.go` - {{.entity}}领域模型
  - `domain/{{.domain}}/model/session.go` - 会话领域模型
  - `domain/{{.domain}}/model/login_attempt.go` - 登录尝试领域模型
- **超时**: 30秒

### 9. 生成GORM模型 (gorm_model_generator)
- **描述**: 生成{{.domain}}相关的GORM数据库模型
- **生成文件**:
  - `internal/infra/persistence/{{.domain}}/model/{{.entity | lower}}_model.go`
  - `internal/infra/persistence/{{.domain}}/model/session_model.go`
  - `internal/infra/persistence/{{.domain}}/model/login_attempt_model.go`
- **{{.entity}}表字段**:
  - `id`: 主键，自增
  - `email`: 唯一索引，非空
  - `password`: 非空，加密存储
  - `status`: 账户状态
  - `failed_login_attempts`: 失败尝试次数
  - `locked_until`: 锁定截止时间
  - `last_login_at`: 最后登录时间
- **超时**: 30秒

### 10. 生成仓库接口 (repository_interface_generator)
- **描述**: 生成{{.domain}}相关的仓库接口
- **生成文件**:
  - `domain/{{.domain}}/repository/{{.entity | lower}}_repository.go`
  - `domain/{{.domain}}/repository/session_repository.go`
  - `domain/{{.domain}}/repository/login_attempt_repository.go`
- **{{.entity}}仓库方法**:
  - `FindByEmail(email string) (*{{.entity}}, error)`
  - `UpdateLoginAttempts(userID uint, attempts int) error`
  - `UpdateLastLogin(userID uint) error`
  - `LockAccount(userID uint, until time.Time) error`
- **超时**: 25秒

### 11. 生成仓库实现 (repository_impl_generator)
- **描述**: 生成{{.domain}}相关的仓库GORM实现
- **生成文件**:
  - `internal/infra/persistence/{{.domain}}/repository/{{.entity | lower}}_repo_gorm.go`
  - `internal/infra/persistence/{{.domain}}/repository/session_repo_gorm.go`
  - `internal/infra/persistence/{{.domain}}/repository/login_attempt_repo_gorm.go`
- **超时**: 35秒

### 12. 生成数据库迁移 (migration_generator)
- **描述**: 生成{{.domain}}相关的数据库迁移文件
- **生成文件**: `internal/infra/database/migration.go`
- **迁移表**: {{.table_name}}, {{.entity | lower}}_sessions, login_attempts
- **超时**: 20秒

### 13. 生成业务用例 (usecase_generator)
- **描述**: 生成{{.entity}}登录业务逻辑层代码
- **生成文件**: `internal/app/{{.domain}}/{{.entity | lower}}.go`
- **业务方法**:
  - `Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error)` - {{.entity}}登录
  - `Logout(ctx context.Context, token string) error` - {{.entity}}登出
  - `RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error)` - 刷新令牌
  - `ValidateSession(ctx context.Context, token string) (*{{.entity}}, error)` - 验证会话
  - `CheckLoginAttempts(ctx context.Context, email string) (bool, error)` - 检查登录尝试
- **超时**: 50秒

### 14. 生成DTO (dto_generator)
- **描述**: 生成{{.domain}}相关的数据传输对象
- **生成文件**: `api/dto/{{.domain | replace "/" "/"}}/{{.entity | lower}}_dto.go`
- **DTO类型**:
  - `LoginRequest` - 登录请求
  - `LoginResponse` - 登录响应
  - `LogoutRequest` - 登出请求
  - `RefreshTokenRequest` - 刷新令牌请求
  - `TokenResponse` - 令牌响应
  - `{{.entity}}Info` - {{.entity}}信息
- **超时**: 25秒

### 15. 生成适配器 (adapter_generator)
- **描述**: 生成{{.domain}}领域模型与DTO之间的适配器
- **生成文件**: `api/adapter/{{.domain | replace "/" "/"}}/{{.entity | lower}}_adapter.go`
- **适配器方法**:
  - `LoginRequestTo{{.entity}}`
  - `{{.entity}}ToLoginResponse`
  - `SessionToTokenResponse`
- **超时**: 20秒

### 16. 生成API路由 (api_generator)
- **描述**: 生成{{.entity}}登录REST API路由
- **生成文件**: `api/http/router.go`
- **API端点**:
  - `POST /api/v1/{{.domain | replace "/" "-"}}/login` - {{.entity}}登录
  - `POST /api/v1/{{.domain | replace "/" "-"}}/logout` - {{.entity}}登出
  - `POST /api/v1/{{.domain | replace "/" "-"}}/refresh` - 刷新令牌
  - `GET /api/v1/{{.domain | replace "/" "-"}}/validate` - 验证会话
- **中间件**: 频率限制、CORS、身份验证
- **超时**: 30秒

### 17. 生成HTTP处理器 (handler_generator)
- **描述**: 生成{{.domain}}相关的HTTP请求处理器
- **生成文件**: `api/http/admin/{{.domain | replace "/" "/"}}/{{.entity | lower}}_handler.go`
- **处理器方法**:
  - `LoginHandler` - 登录处理
  - `LogoutHandler` - 登出处理
  - `RefreshTokenHandler` - 刷新令牌处理
  - `ValidateSessionHandler` - 会话验证处理
- **超时**: 35秒

### 18. 生成gRPC服务 (grpc_generator)
- **描述**: 生成{{.domain}}相关的gRPC服务定义
- **生成文件**: `api/grpc/v1/{{.domain | replace "/" "/"}}/{{.entity | lower}}_service.go`
- **服务方法**:
  - `Login` - 登录服务
  - `Logout` - 登出服务
  - `RefreshToken` - 刷新令牌服务
  - `ValidateSession` - 会话验证服务
- **超时**: 30秒

### 19. 审计日志 (audit_logger)
- **描述**: 设置{{.entity}}登录操作的审计日志记录
- **审计事件**:
  - `{{.domain | replace "/" "_"}}_login_attempt` - 登录尝试
  - `{{.domain | replace "/" "_"}}_login_success` - 登录成功
  - `{{.domain | replace "/" "_"}}_login_failed` - 登录失败
  - `{{.domain | replace "/" "_"}}_logout` - {{.entity}}登出
  - `account_locked` - 账户锁定
  - `suspicious_login_activity` - 可疑登录活动
- **日志保留**: 90天
- **超时**: 15秒

### 20. 生成测试代码 (test_generator)
- **描述**: 生成{{.domain}}相关的单元测试和集成测试代码
- **生成文件**: `test/{{.domain | replace "/" "/"}}/{{.entity | lower}}_test.go`
- **测试类型**:
  - **单元测试**: 业务用例层测试
  - **集成测试**: API层测试
  - **仓库测试**: 数据访问层测试
  - **安全测试**: 认证安全测试
- **测试覆盖**:
  - 登录业务逻辑
  - 密码验证
  - 令牌生成
  - 频率限制
  - 账户锁定
- **超时**: 60秒

### 21. 生成Mock对象 (mock_generator)
- **描述**: 生成{{.domain}}相关的测试用Mock对象
- **生成文件**: `test/mock/{{.domain | replace "/" "/"}}/{{.entity | lower}}_mock.go`
- **Mock接口**:
  - `{{.entity}}Repository`
  - `SessionRepository`
  - `LoginAttemptRepository`
  - `{{.entity}}UseCase`
  - `TokenService`
  - `PasswordService`
- **超时**: 30秒

## 流程配置

- **最大并行步骤**: 3
- **超时策略**: 快速失败
- **重试策略**: 指数退避
- **回滚启用**: 是
- **安全检查启用**: 是

## 输出文件结构

### 领域层 (Domain Layer)
```
domain/{{.domain}}/
├── model/
│   ├── {{.entity | lower}}.go
│   ├── session.go
│   └── login_attempt.go
└── repository/
    ├── {{.entity | lower}}_repository.go
    ├── session_repository.go
    └── login_attempt_repository.go
```

### 基础设施层 (Infrastructure Layer)
```
internal/infra/
├── persistence/{{.domain}}/
│   ├── model/
│   │   ├── {{.entity | lower}}_model.go
│   │   ├── session_model.go
│   │   └── login_attempt_model.go
│   └── repository/
│       ├── {{.entity | lower}}_repo_gorm.go
│       ├── session_repo_gorm.go
│       └── login_attempt_repo_gorm.go
└── database/
    └── migration.go
```

### 应用层 (Application Layer)
```
internal/app/{{.domain}}/
└── {{.entity | lower}}.go
```

### 接口层 (Interface Layer)
```
api/
├── dto/{{.domain}}/
│   └── {{.entity | lower}}_dto.go
├── adapter/{{.domain}}/
│   └── {{.entity | lower}}_adapter.go
├── http/
│   ├── router.go
│   └── admin/{{.domain}}/
│       └── {{.entity | lower}}_handler.go
└── grpc/v1/{{.domain}}/
    └── {{.entity | lower}}_service.go
```

### 测试层 (Test Layer)
```
test/
├── {{.domain}}/
│   └── {{.entity | lower}}_test.go
└── mock/{{.domain}}/
    └── {{.entity | lower}}_mock.go
```

## 安全配置

### 密码策略
- **最小长度**: 8位
- **必须包含**: 大写字母、小写字母、数字
- **特殊字符**: 可选

### 会话策略
- **最大并发会话**: 5个
- **空闲超时**: 1800秒（30分钟）
- **绝对超时**: 86400秒（24小时）

### 频率限制
- **每分钟登录尝试**: 5次
- **每小时登录尝试**: 20次
- **全局频率限制**: 1000次/分钟

### 审计策略
- **记录所有尝试**: 是
- **记录成功登录**: 是
- **记录失败登录**: 是
- **记录账户锁定**: 是
- **日志保留天数**: 90天

## 架构原则

1. **领域驱动设计 (DDD)**: 严格按照DDD四层架构组织代码
2. **依赖注入**: 使用依赖注入管理组件依赖关系
3. **接口隔离**: 通过接口定义清晰的边界
4. **单一职责**: 每个组件只负责单一职责
5. **安全优先**: 内置多层安全防护机制
6. **可测试性**: 提供完整的测试覆盖和Mock支持
7. **可观测性**: 内置审计日志和监控能力
8. **可扩展性**: 支持水平扩展和高并发访问