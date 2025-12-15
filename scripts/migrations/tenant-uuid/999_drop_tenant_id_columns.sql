-- Drops legacy tenant_id columns after UUID-only rollout.
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
\if :{?dry_run}
\else
\set dry_run 'false'
\endif

SET SESSION "tenant_uuid.include_schemas" = :'include_schemas';
SET SESSION "tenant_uuid.exclude_schemas" = :'exclude_schemas';
SET SESSION "tenant_uuid.include_tables" = :'include_tables';
SET SESSION "tenant_uuid.exclude_tables" = :'exclude_tables';
SET SESSION "tenant_uuid.dry_run" = :'dry_run';

DO $$
DECLARE
    include_schemas text[] := NULLIF(regexp_split_to_array(
        coalesce(trim(current_setting('tenant_uuid.include_schemas', true)), ''),
        '\s*,\s*'
    ), ARRAY['']);
    exclude_schemas text[] := NULLIF(regexp_split_to_array(
        coalesce(trim(current_setting('tenant_uuid.exclude_schemas', true)), ''),
        '\s*,\s*'
    ), ARRAY['']);
    include_tables text[] := NULLIF(regexp_split_to_array(
        coalesce(trim(current_setting('tenant_uuid.include_tables', true)), ''),
        '\s*,\s*'
    ), ARRAY['']);
    exclude_tables text[] := NULLIF(regexp_split_to_array(
        coalesce(trim(current_setting('tenant_uuid.exclude_tables', true)), ''),
        '\s*,\s*'
    ), ARRAY['']);
    dry_run boolean := lower(coalesce(current_setting('tenant_uuid.dry_run', true), 'false')) = 'true';
    rec record;
    sql text;
BEGIN
    FOR rec IN
        SELECT table_schema, table_name
        FROM information_schema.columns
        WHERE column_name = 'tenant_id'
          AND (include_schemas IS NULL OR table_schema = ANY(include_schemas))
          AND (exclude_schemas IS NULL OR NOT (table_schema = ANY(exclude_schemas)))
          AND (include_tables IS NULL OR table_name = ANY(include_tables))
          AND (exclude_tables IS NULL OR NOT (table_name = ANY(exclude_tables)))
        ORDER BY table_schema, table_name
    LOOP
        sql := format('ALTER TABLE %I.%I DROP COLUMN tenant_id', rec.table_schema, rec.table_name);
        IF dry_run THEN
            RAISE NOTICE '[DRY-RUN] %', sql;
        ELSE
            EXECUTE sql;
            RAISE NOTICE 'Dropped tenant_id from %.%', rec.table_schema, rec.table_name;
        END IF;
    END LOOP;
END $$;
