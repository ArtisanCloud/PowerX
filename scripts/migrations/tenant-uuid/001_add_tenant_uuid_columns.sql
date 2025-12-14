-- Adds tenant_uuid columns to tables that still have tenant_id.
\if :{?include_schemas}
\else
\set include_schemas ''
\endif
\if :{?exclude_schemas}
\else
\set exclude_schemas 'pg_catalog,information_schema'
\endif
\if :{?include_tables}
\else
\set include_tables ''
\endif
\if :{?exclude_tables}
\else
\set exclude_tables ''
\endif
\if :{?tenant_uuid_type}
\else
\set tenant_uuid_type 'text'
\endif

WITH target AS (
    SELECT table_schema, table_name
    FROM information_schema.columns
    WHERE column_name = 'tenant_id'
      AND (:'include_schemas' = '' OR table_schema = ANY(string_to_array(:'include_schemas', ',')))
      AND NOT (table_schema = ANY(string_to_array(:'exclude_schemas', ',')))
      AND (:'include_tables' = '' OR table_name = ANY(string_to_array(:'include_tables', ',')))
      AND NOT (table_name = ANY(string_to_array(:'exclude_tables', ',')))
    GROUP BY table_schema, table_name
)
SELECT format(
    'ALTER TABLE %I.%I ADD COLUMN IF NOT EXISTS tenant_uuid %s;
     ALTER TABLE %I.%I ALTER COLUMN tenant_uuid TYPE %s USING tenant_uuid::%s;
     COMMENT ON COLUMN %I.%I.tenant_uuid IS %L;',
    table_schema, table_name, :'tenant_uuid_type',
    table_schema, table_name, :'tenant_uuid_type', :'tenant_uuid_type',
    table_schema, table_name,
    'Tenant UUID string introduced by tenant-id migration.'
)
FROM target
\gexec
