-- Adds canonical tenant_uuid column to iam_tenant_key_pairs and backfills data.
-- Run after iam_tenants table is populated with UUID values.

BEGIN;

ALTER TABLE IF EXISTS public.iam_tenant_key_pairs
    ADD COLUMN IF NOT EXISTS tenant_uuid VARCHAR(128);

ALTER TABLE IF EXISTS public.iam_tenant_key_pairs
    ALTER COLUMN tenant_uuid SET DEFAULT '';

UPDATE public.iam_tenant_key_pairs AS kp
SET tenant_uuid = COALESCE(t.uuid::text, '')
FROM public.iam_tenants AS t
WHERE (kp.tenant_uuid IS NULL OR kp.tenant_uuid = '')
  AND kp.tenant_id IS NOT NULL
  AND kp.tenant_id = t.id;

UPDATE public.iam_tenant_key_pairs
SET tenant_uuid = ''
WHERE tenant_uuid IS NULL;

ALTER TABLE IF EXISTS public.iam_tenant_key_pairs
    ALTER COLUMN tenant_uuid SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tenant_key_pairs_scope
    ON public.iam_tenant_key_pairs (env, tenant_uuid);

CREATE UNIQUE INDEX IF NOT EXISTS uk_tenant_key_pairs_scope
    ON public.iam_tenant_key_pairs (env, tenant_uuid, kid)
    WHERE deleted_at IS NULL;

COMMIT;
