import { describe, expect, it } from 'vitest';
import { resourceManagement } from '~/utils/storage-resource-management';
import { catalog } from './fixtures';
import type { StorageRuntime } from '~/composables/api/services/storageRuntimeService';
const option = () => catalog()[2]!;
const runtime = (): StorageRuntime => ({ configured: true, worker_enabled: true, admission_enabled: true, notifications_configured: true, database_cleanup_enabled: false, lifecycle: { configured: true, allow_enforce: true, revision: 1, database_cleanup_enabled: false, sources: [{ ...option().resource, instance_id: 'prod-install', format: 'numeric', supported: true, require_archive: false }], policies: [], runs: [] } });
describe('resource management ownership and truthful execution status', () => {
  it('keeps missing status unknown and unsupported sources observational', () => {
    expect(resourceManagement(option()).state).toBe('unknown');
    const r = runtime(); r.lifecycle!.sources[0]!.supported = false;
    expect(resourceManagement(option(), r)).toMatchObject({ state: 'observe', canConfigure: false });
    r.lifecycle!.sources[0]!.supported = true; r.lifecycle!.configured = false;
    expect(resourceManagement(option(), r)).toMatchObject({ state: 'observe', canConfigure: false });
  });
  it('does not call a disabled policy active; an enabled policy requires worker and enforcement gates', () => {
    const r = runtime(); expect(resourceManagement(option(), r).state).toBe('disabled');
    r.lifecycle!.policies.push({ source_id: option().resource.id, revision: 1, enabled: true, cleanup_mode: 'expire_delete', retention_days: 14, max_bytes: 1024, require_archive: false, owner_instance: 'prod-install', operator_id: 1, updated_at: '2026-11-05T00:00:00Z' });
    r.worker_enabled = false; expect(resourceManagement(option(), r).state).toBe('blocked');
    r.worker_enabled = true; r.lifecycle!.allow_enforce = false; expect(resourceManagement(option(), r).state).toBe('blocked');
    r.lifecycle!.allow_enforce = true; expect(resourceManagement(option(), r).state).toBe('automatic');
    r.lifecycle!.policies[0]!.cleanup_mode = 'disabled'; expect(resourceManagement(option(), r).state).toBe('disabled');
  });
  it('keeps module ownership separate and does not hide an older unconfirmed batch behind a newer completion', () => {
    expect(resourceManagement(catalog()[0]!, runtime())).toMatchObject({ owner: 'backup', state: 'module', canConfigure: false });
    const r = runtime(); r.lifecycle!.runs = [
      { id: 'old', source_id: option().resource.id, state: 'unconfirmed', started_at: '2026-11-01T00:00:00Z' },
      { id: 'new', source_id: option().resource.id, state: 'completed', started_at: '2026-11-05T00:00:00Z' },
    ] as any;
    expect(resourceManagement(option(), r).run?.id).toBe('new'); expect(resourceManagement(option(), r).needsReview).toBe(true);
  });
});
