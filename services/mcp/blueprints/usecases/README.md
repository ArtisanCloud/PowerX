# 用例蓝图文件夹

本文件夹用于管理各种业务用例的蓝图文件，提供标准化的代码生成模板和流程定义。

## 文件夹结构

```
usecases/
├── README.md                    # 本说明文档
├── user_registration_flow.yaml # 用户注册流程蓝图
└── [其他用例蓝图文件]
```

## 蓝图文件命名规范

- 使用 `snake_case` 命名格式
- 文件名应清晰描述用例功能
- 以 `_flow.yaml` 结尾表示这是一个流程蓝图
- 示例：`user_registration_flow.yaml`、`order_management_flow.yaml`

## 现有蓝图说明

### 1. 实体注册流程 (user_registration_flow.md)

**功能描述：** 完整的实体注册功能实现流程，支持自定义领域和实体

**包含步骤：**

- 数据模型生成（可配置实体）
- 数据库迁移文件生成
- Repository 层代码生成
- UseCase 业务逻辑层生成
- API 接口层生成
- 审计日志集成
- 测试代码生成

**适用场景：** 需要实现各种实体注册功能的项目（用户、组织用户、管理员等）

**支持的领域配置：**
- 简单领域：`user`、`admin`、`customer`
- 嵌套领域：`organization/user`、`department/manager`
- 自定义实体：`User`、`OrganizationUser`、`Admin`

## 如何使用蓝图

### 启动 MCP 服务器

```bash
# 启动MCP服务器（通过stdio通信）
go run <path_to_CoreX>/mcp/cmd/main.go
```

### 使用 MCP 工具

MCP 服务器通过 stdio 协议进行通信，可以使用以下工具：

#### 1. 加载蓝图

使用 `load_blueprint` 工具加载指定的蓝图文件：

```json
{
  "tool": "load_blueprint",
  "arguments": {
    "flow_id": "user_registration_flow"
  }
}
```

#### 2. 规划执行计划

使用 `plan_flow` 工具生成执行计划：

```json
{
  "tool": "plan_flow",
  "arguments": {
    "flow_id": "user_registration_flow",
    "variables": {
      "project_name": "MyProject",
      "domain": "user",
      "entity": "User",
      "table_name": "users",
      "author": "Developer"
    }
  }
}
```

#### 3. 渲染代码

使用 `render_plan` 工具渲染最终代码：

```json
{
  "tool": "render_plan",
  "arguments": {
    "plan_id": "generated_plan_id"
  }
}
```

## 创建新的用例蓝图

### 1. 蓝图文件结构

每个蓝图文件应包含以下基本结构：

```yaml
flow_id: "your_flow_name"
name: "流程显示名称"
description: "流程描述"
version: "1.0.0"
author: "作者名称"

# 流程变量
variables:
  project_name:
    type: "string"
    description: "项目名称"
    required: true
  domain:
    type: "string"
    description: "业务领域名称（如: user, organization/user）"
    required: true
  entity:
    type: "string"
    description: "实体名称（如: User, OrganizationUser）"
    required: true
  table_name:
    type: "string"
    description: "数据表名称（如: users, organization_users）"
    required: false

# 执行步骤
steps:
  - id: "step_1"
    name: "步骤名称"
    type: "generator"
    action: "generate_model"
    params:
      # 参数定义
    timeout: 30
    retry: 3
    next: "step_2"
    conditions:
      - field: "success"
        operator: "eq"
        value: true

# 输出定义
outputs:
  files:
    - path: "输出文件路径"
      type: "go"
      description: "文件描述"
```

### 2. 步骤类型说明

- **generator**: 代码生成步骤
- **validator**: 验证步骤
- **transformer**: 数据转换步骤
- **integrator**: 集成步骤

### 3. 常用动作类型和目录位置

**重要说明：** 本蓝图系统是为外部项目使用 CoreX 框架时提供的代码生成模板，生成的代码将放置在外部项目的相应目录中。

#### 数据层生成

- `generate_model`: 生成领域模型 → `domain/<domain>/model/`
- `generate_gorm_model`: 生成 GORM 数据模型 → `internal/infra/persistence/<domain>/<entity>_model.go`
- `generate_migration`: 生成数据库迁移文件 → `internal/infra/database/migration.go`
- `generate_repository_interface`: 生成仓库接口 → `domain/<domain>/repository/`
- `generate_repository_impl`: 生成仓库实现 → `internal/infra/persistence/<domain>/<entity>_repo_gorm.go`

#### 业务层生成

- `generate_usecase`: 生成业务用例 → `internal/app/<domain>/`
- `generate_dto`: 生成数据传输对象 → `api/dto/<domain>/`
- `generate_adapter`: 生成适配器 → `api/adapter/<domain>/`

#### 接口层生成

- `generate_api`: 生成 API 路由定义 → `api/http/router.go`
- `generate_handler`: 生成 HTTP 处理器 → `api/http/<platform>/` (如 admin、web、miniapp、openapi)
- `generate_grpc_service`: 生成 gRPC 服务 → `api/grpc/v1/<domain>_service.go`

#### 测试和工具

- `generate_test`: 生成测试代码 → `test/<domain>/`
- `generate_mock`: 生成 Mock 对象 → `test/mock/<domain>/`

#### 关键说明

1. **Model vs GORM Model**：

   - Domain Model：纯业务领域模型，不包含数据库相关注解，位于 `domain/<domain>/model/`
   - GORM Model：包含数据库映射信息的持久化模型，位于 `internal/infra/persistence/<domain>/`

2. **Repository 层次**：

   - Interface：定义在 domain 层，作为业务契约，位于 `domain/<domain>/repository/`
   - Implementation：实现在 infra 层，继承 BaseRepository，位于 `internal/infra/persistence/<domain>/`

3. **API 层结构**：

   - Handler：处理 HTTP 请求，调用 Logic 层，按平台分类（admin/web/miniapp/openapi）
   - Logic：业务逻辑实现，调用 UseCase 层
   - 使用`make goctl-powerx-apis`命令初始化 handler 和 logic 文件

4. **目录结构原则**：

   - `domain/`：领域层，包含核心业务抽象（model、repository接口、service）
   - `internal/infra/`：基础设施层，包含具体实现（persistence、database、auth等）
   - `internal/app/`：应用层，包含业务用例编排
   - `api/`：接口层，对外提供服务（http、grpc、dto、adapter）

5. **领域化组织**：
   - `<domain>`：代表具体的业务领域名称，支持简单和嵌套路径
   - **简单领域**：如`user`、`order`、`product`、`admin`
   - **嵌套领域**：如`organization/user`、`department/manager`、`system/admin`
   - 支持按业务领域分层组织代码，遵循DDD原则
   - 领域名称应使用小写字母和下划线，如`user_management`、`order_processing`
   - 嵌套领域使用斜杠分隔，生成的路径会自动处理目录结构
   - 每个领域内部保持相同的分层结构，便于维护和扩展
   
   **路径示例**：
   ```
   # 简单领域 domain: "user"
   domain/user/model/user.go
   internal/infra/persistence/user/repository/user_repo_gorm.go
   
   # 嵌套领域 domain: "organization/user"
   domain/organization/user/model/organizationuser.go
   internal/infra/persistence/organization/user/repository/organizationuser_repo_gorm.go
   ```

## 最佳实践

### 1. 模块化设计

- 将复杂流程拆分为多个独立步骤
- 每个步骤职责单一，便于维护
- 步骤间通过条件和依赖关系连接

### 2. 参数化配置

- 使用变量使蓝图更加灵活
- 提供合理的默认值
- 添加详细的参数说明

### 3. 错误处理

- 为每个步骤设置合理的超时时间
- 配置重试机制
- 定义失败时的回滚策略

### 4. 文档完善

- 为每个蓝图提供详细说明
- 包含使用示例和预期输出
- 说明适用场景和限制条件

## 贡献指南

1. **新增蓝图**：按照命名规范创建新的蓝图文件
2. **更新文档**：修改本 README 文件，添加新蓝图的说明
3. **测试验证**：确保蓝图能够正常加载和执行
4. **代码审查**：提交前进行代码审查，确保质量

## 注意事项

- 蓝图文件必须是有效的 YAML 格式
- 步骤 ID 在同一蓝图中必须唯一
- 条件依赖不能形成循环引用
- 生成的代码应遵循项目的编码规范
- 敏感信息不应硬编码在蓝图中

## 相关文档

- [项目规范文档](../../docs/project_spec.md)
- [MCP 服务器文档](../README.md)
- [能力模板文档](../templates/capabilities/README.md)
