# PowerX API 命名与访问规范（全局）

> 本文件定义 PowerX 平台所有 HTTP API 的路径前缀、用途边界、版本策略、鉴权与命名风格。适用于 CoreX 底座、插件框架、插件业务服务。

## 1. 路径前缀与用途边界

### 1.1 公共访问域（对外/客户端）

- **/api/v1/**：对外开放的稳定 API（OpenAPI 可暴露）
  - 典型对象：租户端、开放平台、第三方客户端
  - 版本语义：语义化版本 v1 / v2

- **/api/**：兼容入口（老路径或内部自用），可作为路由代理或重定向到 /api/v1
  - 若 /api/v1 存在同名路径，优先迁移到 /api/v1

> **注意：APIPrefix 可配置**（`cfg.Server.APIPrefix`）。本文档使用 `/api` 作为默认示例，实际运行路径为 `<APIPrefix>/...`，常见取值：`/api` 或 `/api/v1`。

### 1.2 管理/后台域（管理端/控制台）

- **/api/v1/admin/**：管理端 API（带管理权限）
  - 典型对象：管理控制台、运营/内部管理系统
  - 必须带授权 token

### 1.3 内部/宿主域（仅内部使用）

- **<APIPrefix>/internal/**：宿主/插件内部调用入口（不对公网开放）
  - 典型对象：PowerXPlugin Framework、CLI、宿主内部服务
  - **必须最小化暴露，不写入公开 OpenAPI**
  - 允许与 /api/v1 同时存在，但用途必须明确区分

> 说明：已有历史文档/实现中使用 `/internal/*` 或 `/api/internal/*`，统一向 `/api/internal/*` 对齐。

---

## 2. 版本策略

- 稳定对外接口必须挂在 `/api/v1`，有破坏性变更时升级 `/api/v2`
- `/api/internal` 不承诺稳定版本，但变更需记录在变更日志
- `/api` 仅作为兼容入口或内部路由代理，不建议新功能落地

---

## 3. 鉴权与租户透传

- **所有 `/api/v1/admin` 与 `/api/internal` 必须鉴权**
- 租户信息必须通过 token（JWT claims）或 `tenant_uuid` 字段解析，不接受遗留租户头注入。
- 内部接口也需 tenant 校验，禁止跨租户调用

---

## 4. 命名风格

### 4.1 资源命名

- REST 资源采用名词复数：
  - `/api/v1/admin/agents`
  - `/api/v1/admin/knowledge-spaces`

### 4.1.1 插件相关命名

- 管理端插件资源：`/api/v1/admin/plugins/*`
  - 示例：`/api/v1/admin/plugins`、`/api/v1/admin/plugins/:id`
- 宿主内部插件资源：`/api/internal/plugins/*`
  - 示例：`/api/internal/plugins/local/reload`、`/api/internal/plugins/environments/check`
- 插件发布/治理内部分发：`/api/internal/version/*`、`/api/internal/notify/*`
- 宿主模式插件前端入口（反代）：`/_p/<pluginId>/admin/<path>`
  - 示例：`/_p/com.powerx.helloworld/admin/intro`
- 宿主模式插件后端 API（反代）：`/_p/<pluginId>/api/<path>`
  - 示例：`/_p/com.powerx.helloworld/api/healthz`

### 4.2 行为/动作

- 动作用 **子路径** 或 **操作端点**：
  - `/api/v1/admin/agents/:id/activate`
  - `<APIPrefix>/internal/ws-bus/publish`

### 4.3 异步任务

- 提交任务：`POST /.../tasks`
- 查询任务：`GET /.../tasks/:taskId`

---

## 5. OpenAPI / 合同要求

- `/api/v1` 与 `/api/v1/admin` 必须有 OpenAPI 文档
- `/api/internal` 默认不在公开 OpenAPI 中暴露
- 任何新增对外接口必须更新 specs/contracts

---

## 6. 日志 / 追踪 / 审计

- 对外与管理接口必须具备 trace_id
- `/api/internal` 必须记录 tenant/topic/trace_id（若涉及事件）

---

## 7. 示例

### 7.1 对外 API

```
GET /api/v1/knowledge-spaces
```

### 7.2 管理端 API

```
POST /api/v1/admin/agents/test/connection
```

### 7.3 内部 API

```
POST <APIPrefix>/internal/ws-bus/publish
```

### 7.4 插件相关 API

```
GET /api/v1/admin/plugins
POST /api/internal/plugins/local/reload
GET /_p/<pluginId>/admin/
GET /_p/<pluginId>/api/healthz
```

---

## 8. 变更记录

- 2026-02-03：首次定义 `/api/internal` 作为宿主/插件内部 API 前缀
