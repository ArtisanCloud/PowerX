-- Drops legacy tenant_id columns from core Event Fabric tables.
-- Execute after verifying tenant_key/tenant_uuid values are fully populated.

BEGIN;

ALTER TABLE IF EXISTS public.event_topics
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.event_acl_bindings
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.event_envelopes
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.event_delivery_attempts
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.event_dlq_messages
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.event_replay_requests
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE IF EXISTS public.event_subscription_offsets
    DROP COLUMN IF EXISTS tenant_id;

COMMIT;

