-- Consistency report for tenant_uuid rollout.
-- Usage: psql "$DB" -f scripts/ops/checks/tenant_uuid_consistency.sql
\if :{?include_schemas}
\else
\set include_schemas ''
\endif
\if :{?exclude_schemas}
\else
\set exclude_schemas 'pg_catalog,information_schema'
\endif

CREATE TEMP TABLE IF NOT EXISTS tenant_uuid_consistency (
    table_schema text,
    table_name   text,
    has_tenant_id boolean,
    total_rows bigint,
    null_uuid_rows bigint,
    distinct_uuid bigint,
    generated_at timestamptz default now()
);

TRUNCATE tenant_uuid_consistency;

WITH target_tables AS (
    SELECT table_schema, table_name
    FROM information_schema.columns
    WHERE column_name = 'tenant_uuid'
      AND (:'include_schemas' = '' OR table_schema = ANY(string_to_array(:'include_schemas', ',')))
      AND NOT (table_schema = ANY(string_to_array(:'exclude_schemas', ',')))
    GROUP BY table_schema, table_name
)
SELECT format(
    $$INSERT INTO tenant_uuid_consistency(table_schema, table_name, has_tenant_id, total_rows, null_uuid_rows, distinct_uuid)
      SELECT %L, %L,
             EXISTS (
               SELECT 1 FROM information_schema.columns c
               WHERE c.table_schema = %L AND c.table_name = %L AND c.column_name = 'tenant_id'
             ),
             count(*),
             count(*) FILTER (WHERE tenant_uuid IS NULL),
             count(DISTINCT tenant_uuid)
      FROM %I.%I;$$,
    table_schema, table_name, table_schema, table_name, table_schema, table_name
)
FROM target_tables
\gexec

TABLE tenant_uuid_consistency ORDER BY table_schema, table_name;
