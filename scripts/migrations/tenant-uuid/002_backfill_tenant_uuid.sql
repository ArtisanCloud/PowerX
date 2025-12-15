-- Backfills tenant_uuid columns based on tenants directory.
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
\if :{?tenant_table}
\else
\set tenant_table 'public.iam_tenant'
\endif
\if :{?tenant_id_column}
\else
\set tenant_id_column 'id'
\endif
\if :{?tenant_uuid_column}
\else
\set tenant_uuid_column 'uuid'
\endif
\if :{?tenant_uuids}
\else
\set tenant_uuids ''
\endif


SET SESSION "tenant_uuid.include_schemas" = :'include_schemas';
SET SESSION "tenant_uuid.exclude_schemas" = :'exclude_schemas';
SET SESSION "tenant_uuid.include_tables" = :'include_tables';
SET SESSION "tenant_uuid.exclude_tables" = :'exclude_tables';
SET SESSION "tenant_uuid.table" = :'tenant_table';
SET SESSION "tenant_uuid.id_column" = :'tenant_id_column';
SET SESSION "tenant_uuid.uuid_column" = :'tenant_uuid_column';
SET SESSION "tenant_uuid.tenant_uuids" = :'tenant_uuids';

CREATE TABLE IF NOT EXISTS public.tenant_uuid_backfill_report (
    table_schema text,
    table_name   text,
    updated_rows bigint,
    missing_rows bigint,
    executed_at  timestamptz default now()
);

DO $$
DECLARE
    include_schemas text[] := NULLIF(regexp_split_to_array(coalesce(trim(current_setting('tenant_uuid.include_schemas', true)), ''), '\s*,\s*'), ARRAY['']);
    exclude_schemas text[] := NULLIF(regexp_split_to_array(coalesce(trim(current_setting('tenant_uuid.exclude_schemas', true)), ''), '\s*,\s*'), ARRAY['']);
    include_tables text[]  := NULLIF(regexp_split_to_array(coalesce(trim(current_setting('tenant_uuid.include_tables', true)), ''), '\s*,\s*'), ARRAY['']);
    exclude_tables text[]  := NULLIF(regexp_split_to_array(coalesce(trim(current_setting('tenant_uuid.exclude_tables', true)), ''), '\s*,\s*'), ARRAY['']);
    tenant_table text := coalesce(trim(current_setting('tenant_uuid.table', true)), 'public.iam_tenant');
    tenant_id_col text := coalesce(trim(current_setting('tenant_uuid.id_column', true)), 'id');
    tenant_uuid_col text := coalesce(trim(current_setting('tenant_uuid.uuid_column', true)), 'uuid');
    rec record;
    sql text;
    updated bigint;
    missing bigint;
    tenant_filter text[] := NULLIF(regexp_split_to_array(
        coalesce(trim(current_setting('tenant_uuid.tenant_uuids', true)), ''),
        '\s*,\s*'
    ), ARRAY['']);
BEGIN
    FOR rec IN
        SELECT DISTINCT table_schema, table_name
        FROM information_schema.columns AS columns
        WHERE column_name = 'tenant_id'
          AND EXISTS (
              SELECT 1 FROM information_schema.columns c
              WHERE c.table_schema = columns.table_schema
                AND c.table_name = columns.table_name
                AND c.column_name = 'tenant_uuid'
          )
          AND (include_schemas IS NULL OR table_schema = ANY(include_schemas))
          AND (exclude_schemas IS NULL OR NOT (table_schema = ANY(exclude_schemas)))
          AND (include_tables IS NULL OR table_name = ANY(include_tables))
          AND (exclude_tables IS NULL OR NOT (table_name = ANY(exclude_tables)))
        ORDER BY table_schema, table_name
    LOOP
        sql := format('UPDATE %I.%I tgt SET tenant_uuid = t.%I::text
                       FROM %s t
                       WHERE (tgt.tenant_id)::text = t.%I::text
                         AND (tgt.tenant_uuid IS DISTINCT FROM t.%I::text)
                         AND ($1 IS NULL OR t.%I::text = ANY($1))',
                       rec.table_schema, rec.table_name, tenant_uuid_col, tenant_table, tenant_id_col, tenant_uuid_col, tenant_uuid_col);
        EXECUTE sql USING tenant_filter;
        GET DIAGNOSTICS updated = ROW_COUNT;

        sql := format('SELECT count(*)
                       FROM %I.%I tgt
                       JOIN %s t ON (tgt.tenant_id)::text = t.%I::text
                       WHERE tgt.tenant_uuid IS NULL
                         AND ($1 IS NULL OR t.%I::text = ANY($1))',
                      rec.table_schema, rec.table_name, tenant_table, tenant_id_col, tenant_uuid_col);
        EXECUTE sql INTO missing USING tenant_filter;

        INSERT INTO public.tenant_uuid_backfill_report(table_schema, table_name, updated_rows, missing_rows)
        VALUES(rec.table_schema, rec.table_name, updated, missing);

        RAISE NOTICE 'Backfilled %.% (updated %, missing %)', rec.table_schema, rec.table_name, updated, missing;
    END LOOP;
END $$;

SELECT * FROM public.tenant_uuid_backfill_report ORDER BY executed_at DESC, table_schema, table_name LIMIT 100;
