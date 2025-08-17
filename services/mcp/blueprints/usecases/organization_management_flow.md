# 实体管理流程蓝图

## 流程概述

**流程ID**: `entity_management_flow`  
**流程名称**: 实体管理流程  
**描述**: 完整的多实体管理业务流程，支持自定义领域和实体的管理功能，包含CRUD操作、关系管理和权限控制  
**版本**: 1.0.0

## 流程变量

| 变量名 | 类型 | 描述 | 默认值 |
|--------|------|------|--------|
| `project_name` | string | 项目名称 | {{.project_name}} |
| `domain` | string | 领域名称 | {{.domain}} |
| `entities` | array | 实体列表 | {{.entities}} |
| `audit_enabled` | boolean | 是否启用审计 | true |
| `hierarchy_enabled` | boolean | 是否启用层级管理 | true |
| `max_hierarchy_depth` | integer | 最大层级深度 | 10 |

### 数据表配置

| 表名 | 描述 |
|------|------|
| `{{.table_names.primary}}` | 主实体表 |
| `{{.table_names.secondary}}` | 次要实体表 |
| `{{.table_names.relations}}` | 关联表 |

注：具体表名根据实际的实体配置动态生成

## 执行步骤

### 1. 实体层级验证 (hierarchy_validation)

**类型**: validation  
**描述**: 验证实体层级关系的合理性

**验证规则**:
- 最大层级深度检查
- 循环引用检查
- 孤立节点检查
- 实体父子关系验证
- 实体关联分配验证
- 层级一致性验证

**超时**: 10秒  
**重试**: 2次

### 2. 业务规则验证 (business_rule_validation)

**类型**: validation  
**描述**: 验证实体管理的业务规则

**实体规则**:
- 同一父实体下子实体名称唯一
- 实体编码全局唯一
- 实体关联关系验证

**数据规则**:
- 唯一性约束验证
- 数据格式验证
- 状态一致性检查

**关系规则**:
- 关联关系一致性
- 层级关系验证
- 引用完整性检查

**权限规则**:
- 权限一致性验证
- 访问控制检查

**超时**: 15秒  
**重试**: 1次

### 3. 数据完整性检查 (data_integrity_check)

**类型**: validation  
**描述**: 检查实体数据的完整性和一致性

**检查项目**:
- 引用完整性检查
- 数据一致性检查
- 业务约束检查

**超时**: 20秒  
**重试**: 2次

### 4. 生成领域模型 (domain_model_generator)

**类型**: generate  
**描述**: 生成{{.domain}}相关的领域模型

**生成实体**:

#### {{.entities[0]}} (主实体)
- **路径**: `domain/{{.domain}}/model/{{.entities[0] | lower}}.go`
- **字段**:
  - `id` (uint): 实体ID
  - `name` (string): 实体名称
  - `code` (string): 实体编码
  - `parent_id` (*uint): 父实体ID
  - `level` (int): 实体层级
  - `sort_order` (int): 排序
  - `status` (int): 状态
  - `description` (string): 实体描述
  - 其他自定义字段...

#### {{.entities[1]}} (次要实体)
- **路径**: `domain/{{.domain}}/model/{{.entities[1] | lower}}.go`
- **字段**:
  - `id` (uint): 实体ID
  - `name` (string): 实体名称
  - `code` (string): 实体编码
  - `status` (int): 状态
  - 其他自定义字段...

#### 其他实体
- **路径**: `domain/{{.domain}}/model/{{.entity | lower}}.go`
- **字段**: 根据具体实体配置动态生成

注：具体字段根据实际的实体配置动态生成

**超时**: 40秒  
**重试**: 1次

### 5. 生成GORM模型 (gorm_model_generator)

**类型**: generate  
**描述**: 生成{{.domain}}相关的GORM数据库模型

**生成文件**:
- `internal/infra/persistence/{{.domain}}/model/{{.entity | lower}}_model.go` (为每个实体生成)

注：具体文件根据实体配置动态生成

**关系配置**:
- 实体自关联（父子关系）
- 实体间多对多关系
- 实体间一对多关系
- 根据实体配置动态生成关系

**索引配置**:
- 唯一索引：实体编码、关键字段
- 普通索引：外键字段、状态字段、层级字段等
- 根据实体配置动态生成索引

**超时**: 45秒  
**重试**: 1次

### 6. 生成仓库接口 (repository_interface_generator)

**类型**: generate  
**描述**: 生成{{.domain}}相关的仓库接口

**生成文件**:
- `domain/{{.domain}}/repository/{{.entity | lower}}_repository.go` (为每个实体生成)

注：具体文件根据实体配置动态生成

**核心方法**:

#### 主实体仓库
- `FindByParentID()`: 根据父实体ID查找
- `GetEntityTree()`: 获取实体树
- `CheckCircularReference()`: 检查循环引用
- `UpdateHierarchy()`: 更新层级关系

#### 次要实体仓库
- `FindByCode()`: 根据编码查找
- `AssignToEntity()`: 分配到实体
- `UpdateRelation()`: 更新关联关系

#### 通用仓库方法
- `FindByStatus()`: 根据状态查找
- `GetHierarchy()`: 获取层级结构
- `CheckPermission()`: 检查权限
- `FindByPermission()`: 根据权限查找

注：具体方法根据实体配置动态生成

**超时**: 35秒  
**重试**: 1次

### 7. 生成仓库实现 (repository_impl_generator)

**类型**: generate  
**描述**: 生成{{.domain}}相关的仓库GORM实现

**生成文件**:
- `internal/infra/persistence/{{.domain}}/repository/{{.entity | lower}}_repo_gorm.go` (为每个实体生成)

注：具体文件根据实体配置动态生成

**特性**:
- 继承BaseRepository基础功能
- 实现自定义业务方法
- 支持事务操作
- 包含性能优化

**超时**: 50秒  
**重试**: 1次

### 8. 生成数据库迁移 (migration_generator)

**类型**: generate  
**描述**: 生成{{.domain}}相关的数据库迁移文件

**生成文件**: `internal/infra/database/migration.go`

**迁移内容**:
- 创建所有相关表
- 设置外键约束
- 创建索引
- 设置表关系

**外键关系**:
- 实体自关联外键
- 实体间关联外键
- 关联表外键设置
- 根据实体配置动态生成外键关系

**超时**: 30秒  
**重试**: 1次

### 9. 生成业务用例 (usecase_generator)

**类型**: generate  
**描述**: 生成{{.domain}}管理业务逻辑层代码

**生成文件**:
- `internal/app/{{.domain}}/{{.entity | lower}}.go` (为每个实体生成)

注：具体文件根据实体配置动态生成

**核心业务方法**:

#### 实体管理
- `Create{{.entity}}()`: 创建实体
- `Update{{.entity}}()`: 更新实体
- `Delete{{.entity}}()`: 删除实体
- `Get{{.entity}}Tree()`: 获取实体树
- `Move{{.entity}}()`: 移动实体
- `Assign{{.entity}}()`: 分配实体

#### 关系管理
- `AssignToEntity()`: 分配到实体
- `TransferEntity()`: 实体调动
- `UpdateRelation()`: 更新关联关系
- `GetRelatedEntities()`: 获取关联实体

#### 权限管理
- `AssignPermissions()`: 分配权限
- `CheckPermission()`: 检查权限
- `UpdatePermissions()`: 更新权限

注：具体方法根据实体配置动态生成

**超时**: 80秒  
**重试**: 1次

### 10. 生成DTO (dto_generator)

**类型**: generate  
**描述**: 生成{{.domain}}相关的数据传输对象

**生成文件**:
- `api/dto/{{.domain}}/{{.entity | lower}}_dto.go` (为每个实体生成)

注：具体文件根据实体配置动态生成

**DTO类型**:
- Request DTOs: 创建和更新请求
- Response DTOs: 响应数据结构
- List DTOs: 列表查询参数
- Special DTOs: 特殊业务场景

**超时**: 40秒  
**重试**: 1次

### 11. 生成适配器 (adapter_generator)

**类型**: generate  
**描述**: 生成{{.domain}}领域模型与DTO之间的适配器

**生成文件**:
- `api/adapter/{{.domain}}/{{.entity | lower}}_adapter.go` (为每个实体生成)

注：具体文件根据实体配置动态生成

**适配方法**:
- Request到Domain转换
- Domain到Response转换
- 批量转换方法
- 特殊场景转换

**超时**: 30秒  
**重试**: 1次

### 12. 生成API路由 (api_generator)

**类型**: generate  
**描述**: 生成{{.domain}}管理REST API路由

**生成文件**: `api/http/router.go`

**API端点**:

#### 实体管理API
- `GET /api/v1/{{.domain | replace "/" "-"}}/{{.entity | lower}}s` - 实体列表
- `POST /api/v1/{{.domain | replace "/" "-"}}/{{.entity | lower}}s` - 创建实体
- `GET /api/v1/{{.domain | replace "/" "-"}}/{{.entity | lower}}s/{id}` - 获取实体详情
- `PUT /api/v1/{{.domain | replace "/" "-"}}/{{.entity | lower}}s/{id}` - 更新实体
- `DELETE /api/v1/{{.domain | replace "/" "-"}}/{{.entity | lower}}s/{id}` - 删除实体
- `GET /api/v1/{{.domain | replace "/" "-"}}/{{.entity | lower}}s/tree` - 实体树结构
- `PUT /api/v1/{{.domain | replace "/" "-"}}/{{.entity | lower}}s/{id}/move` - 移动实体

#### 员工管理API
- `GET /api/v1/organization/employees` - 员工列表
- `POST /api/v1/organization/employees` - 创建员工
- `GET /api/v1/organization/employees/{id}` - 获取员工详情
- `PUT /api/v1/organization/employees/{id}` - 更新员工
- `DELETE /api/v1/organization/employees/{id}` - 删除员工
- `PUT /api/v1/organization/employees/{id}/transfer` - 员工调动

#### 职位管理API
- `GET /api/v1/organization/positions` - 职位列表
- `POST /api/v1/organization/positions` - 创建职位
- `GET /api/v1/organization/positions/{id}` - 获取职位详情
- `PUT /api/v1/organization/positions/{id}` - 更新职位
- `DELETE /api/v1/organization/positions/{id}` - 删除职位

#### 角色管理API
- `GET /api/v1/organization/roles` - 角色列表
- `POST /api/v1/organization/roles` - 创建角色
- `GET /api/v1/organization/roles/{id}` - 获取角色详情
- `PUT /api/v1/organization/roles/{id}` - 更新角色
- `DELETE /api/v1/organization/roles/{id}` - 删除角色
- `PUT /api/v1/organization/roles/{id}/permissions` - 分配权限

**中间件**:
- `auth_required`: 身份认证
- `permission_check`: 权限检查

**超时**: 50秒  
**重试**: 1次

### 13. 生成HTTP处理器 (handler_generator)

**类型**: generate  
**描述**: 生成组织架构相关的HTTP请求处理器

**生成文件**:
- `api/http/admin/organization/department_handler.go`
- `api/http/admin/organization/employee_handler.go`
- `api/http/admin/organization/position_handler.go`
- `api/http/admin/organization/organization_role_handler.go`

**处理器功能**:
- 请求参数验证
- 业务逻辑调用
- 响应数据转换
- 错误处理
- 日志记录

**超时**: 60秒  
**重试**: 1次

### 14. 生成gRPC服务 (grpc_generator)

**类型**: generate  
**描述**: 生成组织架构相关的gRPC服务定义

**生成文件**: `api/grpc/v1/organization/organization_service.go`

**服务方法**:
- 部门服务方法
- 员工服务方法
- 职位服务方法
- 角色服务方法

**特性**:
- 支持流式传输
- 错误处理
- 元数据传递
- 拦截器支持

**超时**: 40秒  
**重试**: 1次

### 15. 审计日志 (audit_logger)

**类型**: logging  
**描述**: 设置组织架构管理操作的审计日志记录

**审计事件**:

#### 部门操作
- `department_created`: 部门创建
- `department_updated`: 部门更新
- `department_deleted`: 部门删除
- `department_moved`: 部门移动

#### 员工操作
- `employee_created`: 员工创建
- `employee_updated`: 员工更新
- `employee_deleted`: 员工删除
- `employee_transferred`: 员工调动
- `employee_status_changed`: 员工状态变更

#### 职位操作
- `position_created`: 职位创建
- `position_updated`: 职位更新
- `position_deleted`: 职位删除

#### 角色操作
- `role_created`: 角色创建
- `role_updated`: 角色更新
- `role_deleted`: 角色删除
- `permissions_assigned`: 权限分配

#### 安全事件
- `permission_denied`: 权限拒绝
- `unauthorized_access_attempt`: 未授权访问尝试

**日志字段**:
- 操作者ID
- IP地址
- 操作时间
- 操作对象
- 变更内容

**超时**: 20秒  
**重试**: 1次

### 16. 生成测试代码 (test_generator)

**类型**: generate  
**描述**: 生成组织架构相关的单元测试和集成测试代码

**生成文件**:
- `test/organization/department_test.go`
- `test/organization/employee_test.go`
- `test/organization/position_test.go`
- `test/organization/organization_role_test.go`
- `test/organization/organization_integration_test.go`

**测试类型**:

#### 单元测试
- UseCase层测试
- Repository层测试
- 业务逻辑测试

#### 集成测试
- API接口测试
- 数据库集成测试
- 业务流程测试

#### 性能测试
- 大型部门树查询
- 批量员工分配
- 复杂权限检查

**测试覆盖**:
- 核心业务方法
- 边界条件
- 异常情况
- 性能基准

**超时**: 80秒  
**重试**: 1次

### 17. 生成Mock对象 (mock_generator)

**类型**: generate  
**描述**: 生成组织架构相关的测试用Mock对象

**生成文件**:
- `test/mock/organization/department_mock.go`
- `test/mock/organization/employee_mock.go`
- `test/mock/organization/position_mock.go`
- `test/mock/organization/organization_role_mock.go`
- `test/mock/organization/organization_service_mock.go`

**Mock接口**:
- Repository接口
- UseCase接口
- Service接口
- 外部依赖接口

**超时**: 40秒  
**重试**: 1次

## 流程配置

| 配置项 | 值 | 描述 |
|--------|----|---------|
| `max_parallel_steps` | 4 | 最大并行步骤数 |
| `timeout_strategy` | fail_fast | 超时策略 |
| `retry_strategy` | exponential_backoff | 重试策略 |
| `rollback_enabled` | true | 是否启用回滚 |
| `hierarchy_validation_enabled` | true | 是否启用层级验证 |
| `permission_check_enabled` | true | 是否启用权限检查 |

## 输出结构

### 领域层 (Domain Layer)
```
domain/organization/
├── model/
│   ├── department.go
│   ├── employee.go
│   ├── position.go
│   └── organization_role.go
└── repository/
    ├── department_repository.go
    ├── employee_repository.go
    ├── position_repository.go
    └── organization_role_repository.go
```

### 基础设施层 (Infrastructure Layer)
```
internal/infra/
├── persistence/organization/
│   ├── model/
│   │   ├── department_model.go
│   │   ├── employee_model.go
│   │   ├── position_model.go
│   │   └── organization_role_model.go
│   └── repository/
│       ├── department_repo_gorm.go
│       ├── employee_repo_gorm.go
│       ├── position_repo_gorm.go
│       └── organization_role_repo_gorm.go
└── database/
    └── migration.go
```

### 应用层 (Application Layer)
```
internal/app/organization/
├── department.go
├── employee.go
├── position.go
└── organization_role.go
```

### 接口层 (Interface Layer)
```
api/
├── dto/organization/
│   ├── department_dto.go
│   ├── employee_dto.go
│   ├── position_dto.go
│   └── organization_role_dto.go
├── adapter/organization/
│   ├── department_adapter.go
│   ├── employee_adapter.go
│   ├── position_adapter.go
│   └── organization_role_adapter.go
├── http/
│   ├── router.go
│   └── admin/organization/
│       ├── department_handler.go
│       ├── employee_handler.go
│       ├── position_handler.go
│       └── organization_role_handler.go
└── grpc/v1/organization/
    └── organization_service.go
```

### 测试层 (Test Layer)
```
test/
├── organization/
│   ├── department_test.go
│   ├── employee_test.go
│   ├── position_test.go
│   ├── organization_role_test.go
│   └── organization_integration_test.go
└── mock/organization/
    ├── department_mock.go
    ├── employee_mock.go
    ├── position_mock.go
    ├── organization_role_mock.go
    └── organization_service_mock.go
```

## 安全配置

### 权限级别

#### 部门权限
- `organization.department.read`: 部门读取权限
- `organization.department.write`: 部门写入权限
- `organization.department.delete`: 部门删除权限

#### 员工权限
- `organization.employee.read`: 员工读取权限
- `organization.employee.write`: 员工写入权限
- `organization.employee.delete`: 员工删除权限
- `organization.employee.transfer`: 员工调动权限

#### 职位权限
- `organization.position.read`: 职位读取权限
- `organization.position.write`: 职位写入权限
- `organization.position.delete`: 职位删除权限

#### 角色权限
- `organization.role.read`: 角色读取权限
- `organization.role.write`: 角色写入权限
- `organization.role.delete`: 角色删除权限
- `organization.role.assign_permissions`: 权限分配权限

### 数据保护
- 员工个人信息加密
- 薪资信息访问控制
- 审计日志保留
- 敏感操作审批

### 合规要求
- GDPR合规
- 数据保留政策
- 访问日志监控
- 隐私保护

## 架构原则

1. **领域驱动设计 (DDD)**: 严格遵循四层架构
2. **单一职责**: 每个组件职责明确
3. **依赖倒置**: 高层模块不依赖低层模块
4. **接口隔离**: 接口设计精简专一
5. **开闭原则**: 对扩展开放，对修改关闭
6. **数据一致性**: 确保组织架构数据的一致性
7. **性能优化**: 针对层级查询进行优化
8. **安全第一**: 全面的权限控制和审计

## 成功条件

- ✅ 所有验证步骤通过
- ✅ 代码生成完成且无语法错误
- ✅ 数据库迁移文件正确
- ✅ 测试覆盖率达到80%以上
- ✅ API文档生成完整
- ✅ 安全配置正确应用
- ✅ 审计日志正常记录

## 错误处理

- **验证失败**: 停止流程，返回详细错误信息
- **生成失败**: 重试机制，记录错误日志
- **超时处理**: 根据配置进行重试或失败处理
- **回滚机制**: 支持部分回滚和完全回滚

## 依赖关系

- **Go 1.19+**: 编程语言
- **GORM**: ORM框架
- **Gin**: HTTP框架
- **gRPC**: RPC框架
- **PostgreSQL/MySQL**: 数据库
- **Redis**: 缓存
- **Testify**: 测试框架
- **Mockery**: Mock生成工具