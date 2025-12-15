-- Drops legacy tenant_id columns from event fabric authorization tables.
-- Execute after confirming tenant_uuid columns are fully populated.

BEGIN;

ALTER TABLE IF EXISTS public.event_auth_grants
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.event_auth_approval_tickets
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.event_auth_grant_templates
    DROP COLUMN IF EXISTS tenant_id;

COMMIT;

