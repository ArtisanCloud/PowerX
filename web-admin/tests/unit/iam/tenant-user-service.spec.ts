import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useTenantUserService } from '~/composables/api/services/tenantUserService';

const client = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), patch: vi.fn(), put: vi.fn(), delete: vi.fn() }));
vi.mock('~/composables/api/index', () => ({ useApiClient: () => client }));
beforeEach(() => vi.clearAllMocks());

describe('tenant user API boundary', () => {
  it('creates a member and roles together without forwarding a caller selected tenant', async () => {
    await useTenantUserService().createUser({ tenant_uuid: 'ignored-tenant', username: 'person', display_name: 'Person', email: 'person@example.invalid', initial_password: 'fixture-password', role_uuids: ['role-user'], dept_ids: [3], status: 1 });
    expect(client.post).toHaveBeenCalledWith('/admin/iam/members', {
      member: { username: 'person', display_name: 'Person', avatar_url: undefined, status: 1, meta: undefined },
      user: { email: 'person@example.invalid', phone: undefined },
      initial_password: 'fixture-password', dept_ids: [3], role_uuids: ['role-user'],
    });
  });
  it('uses the member UUID for every related operation and keeps tenant status local', async () => {
    const api = useTenantUserService();
    await api.getUsers({ tenant_uuid: 'ignored', page: 2, page_size: 10, status: 0 });
    expect(client.get).toHaveBeenCalledWith('/admin/iam/members', { params: { page: 2, page_size: 10, status: 2 } });
    await api.updateUser('member-uuid', { tenant_uuid: 'ignored', username: 'edited', role_uuids: ['role-admin'] });
    expect(client.patch).toHaveBeenCalledWith('/admin/iam/members/member-uuid', expect.objectContaining({ member: expect.objectContaining({ username: 'edited' }), role_uuids: ['role-admin'] }));
    await api.updateUser('member-uuid', { tenant_uuid: 'ignored', username: 'edited', dept_ids: [] });
    expect(client.patch).toHaveBeenLastCalledWith('/admin/iam/members/member-uuid', expect.objectContaining({ dept_ids: [] }));
    await api.getUsers({ tenant_uuid: 'ignored', dept_id: 9, recursive: true });
    expect(client.get).toHaveBeenLastCalledWith('/admin/iam/members', { params: { dept_id: 9, recursive: true } });
    await api.getUserRoles('member-uuid', { tenant_uuid: 'ignored' });
    expect(client.get).toHaveBeenLastCalledWith('/admin/iam/members/member-uuid/roles');
    await api.setUserStatus('member-uuid', { status: 0 });
    expect(client.put).toHaveBeenLastCalledWith('/admin/iam/members/member-uuid/status', { status: 2 });
    await api.resetUserPassword('member-uuid', { new_password: 'fixture-password' });
    expect(client.put).toHaveBeenLastCalledWith('/admin/iam/members/member-uuid/password', { new_password: 'fixture-password' });
    await api.deleteUser('member-uuid');
    expect(client.delete).toHaveBeenCalledWith('/admin/iam/members/member-uuid');
    for (const mock of Object.values(client)) {
      for (const [path, payload] of mock.mock.calls) {
        expect(path).not.toContain('/system/users');
        expect(payload ?? {}).not.toHaveProperty('tenant_uuid');
        expect(payload ?? {}).not.toHaveProperty('user_uuid');
      }
    }
  });
});
