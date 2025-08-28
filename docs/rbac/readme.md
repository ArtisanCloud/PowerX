#README（含 Postgres 视图与常用 SQL）。

我把我们这轮做的“后端接口 + 种子数据 + 前端对接 + 权限归类”完整串了一遍，后面紧跟一组 CREATE VIEW / FUNCTION，方便你用 SQL 检查和管理 RBAC 数据。

---

# RBAC 实现说明（PowerX CoreX）

## TL;DR

* **模型**：`iam_permission`、`iam_role`、`iam_role_permission`、`iam_role_binding`、`iam_member`、`iam_user`。
* **权限注册**

    * 手动/批量注册（`RegisterPermissions` / `SyncPermissions`）。
    * Swagger→权限：按路由自动生成 `plugin=core` 的 API 权限（`SeedSwaggerPermissions`）。
* **权限归类**：以 `permission.meta` 里的 `module`、`type` 映射到前端树（module→type→items）。

    * API：`type=api`，携带 `api_endpoint`、`http_method`；
    * 非 API：`type=action`（或种子里自定义）。
* **角色-权限**

    * 查询角色权限：`GET /api/admin/iam/roles/:id/permissions`
    * 一次性设置整套权限：`PUT /api/admin/iam/roles/:id/permissions/set-ids`，`{ "ids":[...] }`
* **鉴权视角**

    * Root：不受租户限制，可管理 System 角色；
    * 租户：仅能操作本租户，且不能操作系统角色（若启用该限制）。
* **前端对接**

    * `stores/permission.ts` 支持“拉全量 + 本地筛选 + 批量提交 set-ids”；
    * `PermissionManager.vue`：模块/类型分组勾选、整页保存。
* **种子（seed）**

    * `SeedSystemBuiltinRoles`：`system_admin` / `system_monitor`（系统级）；
    * `SeedSystemPermissions`：IAM 基础 action 权限（`iam.role/read|write|delete|bind` 等）；
    * `SeedSwaggerPermissions`：从 `./docs/swagger.json` 导入 API 权限；
    * `SeedBuiltInRolesAndGrants(tenantID)`：

        * Upsert `root`(system, tenant\_id=0) 和 `tenant_admin`(tenant, tenant\_id=指定)；
        * **root** 授予**全部 active** 权限；
        * `tenant_admin` 授予**非系统模块**的 active 权限（过滤 `meta->>'module'='system'`）。
    * `EnsureDefaultRoles(tenant)` + `SeedGrantDefaultRolesForTenant(tenant)`：

        * `role_admin`：授予**所有**权限；
        * `role_user`：授予 `action='read'` 的只读权限。
* **重要细节**

    * `iam_role_permission.created_at`：批量插入时使用结构体或在 SQL 层使用 `NOW()`，已避免 `NULL`。
    * `BaseRepository.GetFirst`：返回 `(*T, nil)` 或 `nil, nil`，调用方要判空。
    * `SetPermissionIDs`：只允许 `status=active` 的权限，跳过 deprecated，事务中“增/删”幂等。
    * **唯一键**：

        * `iam_permission (plugin,resource,action)`
        * `iam_role (scope,tenant_id,code)`
        * `iam_role_permission PK(role_id,permission_id)`

---

## API 一览

* **权限**

    * `GET /api/admin/iam/permissions` —— 支持 `status、page、page_size、sort` 等查询；拉全量用 `page_size=1000`。
    * `GET /api/admin/iam/permissions/catalog` —— 返回 `module→type→[]permission` 树。
    * `POST /api/admin/iam/permissions/register` —— 手工注册/批量 UPSERT。
    * `POST /api/admin/iam/permissions/sync` —— 源同步（支持 dry-run）。

* **角色**

    * `GET /api/admin/iam/roles`、`POST /api/admin/iam/roles`、`PATCH /api/admin/iam/roles/:id`、`DELETE /api/admin/iam/roles/:id`
    * `GET /api/admin/iam/roles/:id/permissions` —— 列表（用于勾选初始化）。
    * `PUT /api/admin/iam/roles/:id/permissions/set-ids` —— 一次性设置整套权限。

        * 响应：`{ added:[], removed:[], now:[], skipped_deprecated:[] }`

---

## Swagger → Permission 归类规则

* **resource**：来自路径前两段（去 `api` / `admin` 前缀；`:id`/`{id}` 记为字面），如 `/api/admin/iam/departments/:id` → `iam.department`；
* **action**：

    * `GET` → `list|read`（包含 `/:id` 视为 `read`）
    * `POST` → `create`
    * `PUT/PATCH` → `update`
    * `DELETE` → `delete`
* **meta**（JSON）：

  ```json
  {
    "type": "api",
    "module": "api",
    "label": "METHOD /path",
    "http_method": "GET|POST|...",
    "api_endpoint": "/api/..."
  }
  ```

---

## 前端（Pinia + Vue）接入约定

* 初始化：`GET /permissions?page=1&page_size=1000&status=active` 拉全量 → 前端本地按 `module/type` 分组。
* 选中角色：`GET /roles/:id/permissions` → 本地 `roleSelection[roleId]=[ids...]`。
* 勾选并保存：“保存”触发 `PUT /roles/:id/permissions/set-ids`，一次性提交整套权限 ID。

---

# Postgres 视图与常用 SQL

> 以下语句默认 schema 为 `public`，表名为：`iam_permission` / `iam_role` / `iam_role_permission` / `iam_role_binding` / `iam_member`。如你的 schema/表名不同，请自行替换。

## 1) 扁平化权限视图（展开 meta）

```sql
CREATE OR REPLACE VIEW public.vw_iam_permission_flat AS
SELECT
  p.id,
  p.plugin,
  p.resource,
  p.action,
  p.effect,
  p.status,
  p.source,
  p.introduced,
  p.deprecated_at,
  p.created_at,
  p.updated_at,
  /* meta 展开 */
  p.meta->>'label'        AS label,
  p.meta->>'module'       AS module,
  COALESCE(p.meta->>'type','action') AS type,
  p.meta->>'api_endpoint' AS api_endpoint,
  p.meta->>'http_method'  AS http_method
FROM public.iam_permission p;
```

## 2) 角色-权限明细视图（含权限字段）

```sql
CREATE OR REPLACE VIEW public.vw_iam_role_permission_detail AS
SELECT
  r.id             AS role_id,
  r.code           AS role_code,
  r.name           AS role_name,
  r.scope          AS role_scope,
  r.tenant_id      AS role_tenant_id,
  rp.permission_id,
  p.plugin,
  p.resource,
  p.action,
  p.status,
  p.effect,
  /* 展开 meta */
  p.meta->>'module'       AS module,
  COALESCE(p.meta->>'type','action') AS type,
  p.meta->>'http_method'  AS http_method,
  p.meta->>'api_endpoint' AS api_endpoint,
  p.meta->>'label'        AS label
FROM public.iam_role_permission rp
JOIN public.iam_role r       ON r.id = rp.role_id
JOIN public.iam_permission p ON p.id = rp.permission_id;
```

## 3) 成员-角色绑定视图

```sql
CREATE OR REPLACE VIEW public.vw_iam_member_role_bindings AS
SELECT
  rb.tenant_id,
  rb.subject_type,
  rb.subject_id AS member_id,
  m.user_id,
  r.id          AS role_id,
  r.code        AS role_code,
  r.name        AS role_name,
  r.scope       AS role_scope
FROM public.iam_role_binding rb
JOIN public.iam_member m ON m.id = rb.subject_id
JOIN public.iam_role   r ON r.id = rb.role_id
WHERE rb.subject_type = 'MEMBER';
```

> 如果你的 `subject_type` 常量不同，请替换为模型常量对应的字符串。

## 4) 成员生效权限视图（通过角色推导）

```sql
CREATE OR REPLACE VIEW public.vw_iam_member_permissions AS
SELECT DISTINCT
  mr.tenant_id,
  mr.member_id,
  mr.user_id,
  rpd.permission_id,
  pf.plugin,
  pf.resource,
  pf.action,
  pf.type,
  pf.module,
  pf.http_method,
  pf.api_endpoint,
  pf.status
FROM public.vw_iam_member_role_bindings mr
JOIN public.iam_role_permission rpp ON rpp.role_id = mr.role_id
JOIN public.vw_iam_permission_flat pf ON pf.id = rpp.permission_id
JOIN public.iam_role_permission rpd ON rpd.role_id = mr.role_id AND rpd.permission_id = pf.id;
```

## 5) 角色授予统计视图

```sql
CREATE OR REPLACE VIEW public.vw_iam_role_perm_counts AS
SELECT
  r.id   AS role_id,
  r.code AS role_code,
  r.name AS role_name,
  r.scope AS role_scope,
  r.tenant_id AS role_tenant_id,
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE pf.type = 'api')     AS api_count,
  COUNT(*) FILTER (WHERE pf.type = 'action')  AS action_count,
  COUNT(*) FILTER (WHERE pf.status = 'deprecated') AS deprecated_count
FROM public.iam_role r
LEFT JOIN public.iam_role_permission rp ON rp.role_id = r.id
LEFT JOIN public.vw_iam_permission_flat pf ON pf.id = rp.permission_id
GROUP BY r.id, r.code, r.name, r.scope, r.tenant_id;
```

## 6) 角色差集（诊断函数，可选）

> 视图不支持参数，这里提供一个 **函数**，用于对比两个角色的权限差集。

```sql
CREATE OR REPLACE FUNCTION public.fn_role_perm_diff(
  role_id_a BIGINT,
  role_id_b BIGINT
) RETURNS TABLE(
  permission_id BIGINT,
  plugin TEXT,
  resource TEXT,
  action TEXT,
  type TEXT,
  module TEXT,
  http_method TEXT,
  api_endpoint TEXT,
  status TEXT
) LANGUAGE sql STABLE AS
$$
  SELECT
    pf.id,
    pf.plugin, pf.resource, pf.action,
    pf.type, pf.module, pf.http_method, pf.api_endpoint,
    pf.status
  FROM public.iam_role_permission rp
  JOIN public.vw_iam_permission_flat pf ON pf.id = rp.permission_id
  WHERE rp.role_id = role_id_a
    AND NOT EXISTS (
      SELECT 1 FROM public.iam_role_permission rp2
      WHERE rp2.role_id = role_id_b
        AND rp2.permission_id = rp.permission_id
    )
  ORDER BY module NULLS LAST, plugin, resource, action;
$$;
```

**用法：**

```sql
SELECT * FROM public.fn_role_perm_diff( /*root_id*/ 1, /*role_admin_id*/ 2 );
```

---

## 建议索引（如未建立）

```sql
-- 权限唯一：plugin + resource + action
CREATE UNIQUE INDEX IF NOT EXISTS ux_iam_permission_p_r_a
  ON public.iam_permission (plugin, resource, action);

-- 角色唯一：scope + tenant_id + code
CREATE UNIQUE INDEX IF NOT EXISTS ux_iam_role_scope_tenant_code
  ON public.iam_role (scope, tenant_id, code);

-- 角色-权限PK
ALTER TABLE public.iam_role_permission
  ADD CONSTRAINT pk_iam_role_permission PRIMARY KEY (role_id, permission_id);

-- 常用查询加速
CREATE INDEX IF NOT EXISTS ix_iam_role_permission_role
  ON public.iam_role_permission (role_id);

CREATE INDEX IF NOT EXISTS ix_iam_permission_status
  ON public.iam_permission (status);

-- meta JSONB 提取字段的表达式索引（按需开启）
CREATE INDEX IF NOT EXISTS ix_iam_permission_meta_module
  ON public.iam_permission ( (meta->>'module') );

CREATE INDEX IF NOT EXISTS ix_iam_permission_meta_type
  ON public.iam_permission ( (meta->>'type') );
```

---

## 典型管理 SQL（示例）

* **查看某角色拥有的权限数与分类：**

```sql
SELECT * FROM public.vw_iam_role_perm_counts WHERE role_id = 2;  -- 改成你的角色ID
```

* **列出某角色的所有 API 权限：**

```sql
SELECT *
FROM public.vw_iam_role_permission_detail
WHERE role_id = 2 AND type = 'api'
ORDER BY module, api_endpoint;
```

* **比较 root 与某租户的 role\_admin 差异（root 独有）：**

```sql
SELECT *
FROM public.fn_role_perm_diff(1, 2);  -- 1=root_id, 2=role_admin_id
```

* **某成员在某租户的“生效”权限：**

```sql
SELECT *
FROM public.vw_iam_member_permissions
WHERE tenant_id = 1 AND member_id = 123  -- 改成你的 ID
ORDER BY module, plugin, resource, action;
```

---

## 种子执行顺序参考

1. 迁移：创建表 + 约束 + 索引。
2. `SeedSystemBuiltinRoles`（系统级角色）。
3. `SeedSystemPermissions`（IAM action 权限）。
4. `SeedSwaggerPermissions`（导入 API 权限）。
5. `EnsureByKey(system)` 创建/确保 system 租户。
6. `SeedBuiltInRolesAndGrants(system_tenant_id)`：

    * upsert `root` / `tenant_admin`；
    * 授予 root=全量、tenant\_admin=非 system 模块；
7. `EnsureDefaultRoles(system_tenant_id)` + `SeedGrantDefaultRolesForTenant(system_tenant_id)`：

    * `role_admin`=全量、`role_user`=只读；
8. 创建 `root` 用户/凭证 → 创建 system 租户下的 `root` 成员 → 绑定 `role_admin`；
9. 前端拉取全量权限 & 角色，按页面勾选后 `set-ids` 提交。

---