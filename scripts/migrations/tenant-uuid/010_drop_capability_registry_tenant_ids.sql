-- Drops legacy tenant_id columns from capability registry tables.
-- Run after verifying tenant_uuid columns are populated.

BEGIN;

ALTER TABLE IF EXISTS public.capability_registry_registrations
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.capability_registry_adapter_endpoints
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.capability_registry_discovery_cache
    DROP COLUMN IF EXISTS tenant_id;

COMMIT;
