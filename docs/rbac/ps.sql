-- ============================================
-- RBAC helper views / functions for PostgreSQL
-- Safe & idempotent (CREATE OR REPLACE + IF NOT EXISTS)
-- ============================================

-- 建议：确保 search_path 指向 public（按你库实际情况调整）
SET search_path TO public;

-- ========== 视图：权限平铺，展开 meta ==========
CREATE OR REPLACE VIEW vw_iam_permission_flat AS
SELECT
    p.id,
    p.plugin,
    p.resource,
    p.action,
    p.effect,
    p.description,
    p.status,
    p.source,
    p.introduced,
    p.deprecated_at,
    COALESCE(p.meta->>'module', p.plugin)       AS module,
  COALESCE(p.meta->>'type',   'action')       AS type,
  p.meta->>'label'                             AS label,
  p.meta->>'http_method'                       AS http_method,
  p.meta->>'api_endpoint'                      AS api_endpoint
FROM iam_permission AS p;

COMMENT ON VIEW vw_iam_permission_flat IS 'iam_permission 展开 meta 字段（module/type/http_method/api_endpoint）';

-- ========== 视图：角色-权限 明细 ==========
CREATE OR REPLACE VIEW vw_iam_role_permission_detail AS
SELECT
    r.id                                 AS role_id,
    r.scope                              AS role_scope,
    r.tenant_id                          AS role_tenant_id,
    r.code                               AS role_code,
    r.name                               AS role_name,
    r.builtin                            AS role_builtin,
    rp.permission_id,
    rp.created_at                        AS granted_at,
    v.plugin, v.resource, v.action, v.effect, v.description, v.status,
    v.module, v.type, v.label, v.http_method, v.api_endpoint
FROM iam_role_permission AS rp
         JOIN iam_role                AS r ON r.id = rp.role_id
         JOIN vw_iam_permission_flat  AS v ON v.id = rp.permission_id;

COMMENT ON VIEW vw_iam_role_permission_detail IS '角色-权限 明细（包含权限 meta 展开列）';

-- ========== 视图：成员-角色 绑定 ==========
-- 说明：如果你的 subject_type 不是 'MEMBER' 文本，而是数字枚举，请把条件改成相应值
CREATE OR REPLACE VIEW vw_iam_member_role_bindings AS
SELECT
    rb.tenant_id,
    rb.role_id,
    rb.subject_id                AS member_id,
    m.user_id,
    m.username,
    m.display_name,
    r.code                       AS role_code,
    r.name                       AS role_name,
    r.scope                      AS role_scope,
    r.builtin                    AS role_builtin
FROM iam_role_binding AS rb
         JOIN iam_member       AS m ON m.id = rb.subject_id AND m.tenant_id = rb.tenant_id
         JOIN iam_role         AS r ON r.id = rb.role_id
WHERE rb.subject_type = 'MEMBER';

COMMENT ON VIEW vw_iam_member_role_bindings IS '成员-角色 绑定（仅 subject_type=MEMBER）';

-- ========== 视图：成员最终权限（去重） ==========
CREATE OR REPLACE VIEW vw_iam_member_permissions AS
SELECT DISTINCT
    mrb.tenant_id,
    mrb.member_id,
    mrb.user_id,
    mrb.role_id,
    v.id             AS permission_id,
    v.plugin,
    v.resource,
    v.action,
    v.module,
    v.type,
    v.label,
    v.http_method,
    v.api_endpoint,
    v.status
FROM vw_iam_member_role_bindings AS mrb
         JOIN iam_role_permission        AS rp ON rp.role_id = mrb.role_id
         JOIN vw_iam_permission_flat     AS v  ON v.id       = rp.permission_id
WHERE COALESCE(v.status, 'active') = 'active';

COMMENT ON VIEW vw_iam_member_permissions IS '成员的最终权限（经由角色绑定 + 去重，仅 active）';

-- ========== 视图：每个角色的权限数量 ==========
CREATE OR REPLACE VIEW vw_iam_role_perm_counts AS
SELECT
    r.id          AS role_id,
    r.scope       AS role_scope,
    r.tenant_id   AS role_tenant_id,
    r.code        AS role_code,
    r.name        AS role_name,
    r.builtin     AS role_builtin,
    COUNT(DISTINCT rp.permission_id) AS perm_count
FROM iam_role AS r
         LEFT JOIN iam_role_permission AS rp ON rp.role_id = r.id
GROUP BY r.id, r.scope, r.tenant_id, r.code, r.name, r.builtin;

COMMENT ON VIEW vw_iam_role_perm_counts IS '每个角色的去重权限数量统计';

-- ========== 函数：对比两个角色的权限差集 ==========
-- 用法：SELECT * FROM fn_role_perm_diff(<role_a_id>, <role_b_id>);
CREATE OR REPLACE FUNCTION fn_role_perm_diff(a_role_id BIGINT, b_role_id BIGINT)
RETURNS TABLE(
  permission_id BIGINT,
  in_a BOOLEAN,
  in_b BOOLEAN,
  plugin TEXT,
  resource TEXT,
  action TEXT,
  http_method TEXT,
  api_endpoint TEXT
)
LANGUAGE SQL
AS $$
  WITH
  a AS (SELECT DISTINCT permission_id FROM iam_role_permission WHERE role_id = a_role_id),
  b AS (SELECT DISTINCT permission_id FROM iam_role_permission WHERE role_id = b_role_id),
  u AS (
    SELECT
      COALESCE(a.permission_id, b.permission_id) AS pid,
      (a.permission_id IS NOT NULL) AS in_a,
      (b.permission_id IS NOT NULL) AS in_b
    FROM a
    FULL OUTER JOIN b ON a.permission_id = b.permission_id
  )
SELECT
    u.pid AS permission_id,
    u.in_a,
    u.in_b,
    p.plugin,
    p.resource,
    p.action,
    p.meta->>'http_method'  AS http_method,
    p.meta->>'api_endpoint' AS api_endpoint
FROM u
    JOIN iam_permission AS p ON p.id = u.pid
ORDER BY p.plugin, p.resource, p.action;
$$;

COMMENT ON FUNCTION fn_role_perm_diff(BIGINT, BIGINT) IS '比较两个角色权限：列出并标记是否在 A/B 中';

-- ========== 索引（若不存在则创建） ==========
-- 角色权限（幂等约束/查询）
CREATE UNIQUE INDEX IF NOT EXISTS ux_iam_role_permission
    ON iam_role_permission(role_id, permission_id);

CREATE INDEX IF NOT EXISTS idx_iam_role_permission_perm
    ON iam_role_permission(permission_id);

-- 角色（确保 (scope, tenant_id, code) 唯一）
CREATE UNIQUE INDEX IF NOT EXISTS ux_iam_role_scope_tenant_code
    ON iam_role(scope, tenant_id, code);

-- 权限：按状态查询 + meta(GIN)
CREATE INDEX IF NOT EXISTS idx_iam_permission_status
    ON iam_permission(status);

-- 如果 meta 是 jsonb，创建 GIN 索引（文本 path 查询更快）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname = current_schema()
      AND indexname  = 'idx_iam_permission_meta_gin'
  ) THEN
    EXECUTE 'CREATE INDEX idx_iam_permission_meta_gin ON iam_permission USING GIN ((meta));';
END IF;
END$$;

-- 角色绑定：避免重复绑定
CREATE UNIQUE INDEX IF NOT EXISTS ux_iam_role_binding_unique
    ON iam_role_binding(tenant_id, role_id, subject_type, subject_id);

-- ========== 一些常用查询模板（注释掉，拷走备用） ==========
-- -- 1) 查看某角色的权限（含 API 信息）
-- -- SELECT * FROM vw_iam_role_permission_detail WHERE role_id = 7 ORDER BY plugin, resource, action;

-- -- 2) 比较两个角色差异（A 有 B 没有 / 交集 / B 有 A 没有）
-- -- A 独有：SELECT * FROM fn_role_perm_diff(7, 6) WHERE in_a AND NOT in_b;
-- -- 交集：  SELECT * FROM fn_role_perm_diff(7, 6) WHERE in_a AND in_b;
-- -- B 独有：SELECT * FROM fn_role_perm_diff(7, 6) WHERE in_b AND NOT in_a;

-- -- 3) 某成员最终权限
-- -- SELECT * FROM vw_iam_member_permissions WHERE tenant_id = 1 AND member_id = 42 ORDER BY plugin, resource, action;

-- -- 4) 每个角色的权限数量
-- -- SELECT * FROM vw_iam_role_perm_counts ORDER BY perm_count DESC, role_code;

-- 完成
