import { mount, flushPromises } from '@vue/test-utils';
import { createPinia } from 'pinia';
import { defineComponent, h, ref } from 'vue';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import UsersTenantAdmin from '~/components/settings/users/UsersTenantAdmin.vue';
import zh from '../../../i18n/locales/zh.json';

const api = vi.hoisted(() => ({ list: vi.fn(), roles: vi.fn(), remove: vi.fn(), create: vi.fn(), update: vi.fn(), status: vi.fn(), password: vi.fn(), rootCreate: vi.fn(), setRoles: vi.fn() }));
vi.mock('~/composables/api/services/userService', () => ({
  useUserService: () => ({ getUsers: api.list, getUserRoles: api.roles, deleteUser: api.remove, createSystemUser: api.rootCreate, updateUser: api.update, setUserRoles: api.setRoles }),
}));
vi.mock('~/composables/api/services/tenantUserService', () => ({
  useTenantUserService: () => ({ getUsers: api.list, getUserRoles: api.roles, deleteUser: api.remove, createUser: api.create, updateUser: api.update, setUserStatus: api.status, resetUserPassword: api.password }),
}));
vi.mock('~/composables/api/services/roleService', () => ({
  useRoleService: () => ({ getRoles: async () => ({ data: { items: [{ uuid: "role-user", name: "Tenant User", code: "role_user" }] } }) }),
}));
vi.mock('~/stores/department', async () => {
  const { defineStore } = await import('pinia');
  return { useDepartmentStore: defineStore('pagination-department', {
    state: () => ({ tree: [{ id: 1, name: "Engineering", status: 1, children: [{ id: 2, name: "Platform", status: 1 }] }] }), actions: { fetchTree: async () => {} },
  }) };
});
vi.mock('~/stores/user', async () => {
  const { defineStore } = await import('pinia');
  return { useUserStore: defineStore('pagination-user', {
    state: () => ({ user: { uuid: 'actor' }, isRoot: false, isCurrentTenantAdmin: true }),
  }) };
});
const t = (key: string, vars: Record<string, unknown> = {}) => {
  let value = key.split('.').reduce((part: any, name) => part?.[name], zh) ?? key;
  for (const [name, replacement] of Object.entries(vars)) value = value.replaceAll(`{${name}}`, String(replacement));
  return value;
};
vi.mock('#imports', () => ({
  useI18n: () => ({ t: (...args: Parameters<typeof t>) => t(...args), locale: ref('zh') }),
  useToast: () => ({ add: vi.fn() }),
}));

const stubs = {
  UInput: { props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' },
  USelect: defineComponent({
    props: ['modelValue', 'items', 'disabled'], emits: ['update:modelValue'],
    setup: (props, { emit, attrs }) => () => h('select', {
      ...attrs, value: props.modelValue ?? '', disabled: props.disabled,
      onChange: (event: Event) => emit('update:modelValue', props.items.find((item: any) => String(item.value ?? '') === (event.target as HTMLSelectElement).value)?.value),
    }, props.items.map((item: any) => h('option', { value: item.value ?? '' }, item.label))),
  }),
  UButton: { props: ['disabled'], template: '<button :disabled="disabled"><slot /></button>' },
  UFormField: { template: '<div><slot /></div>' },
  UDropdownMenu: { template: '<div><slot /></div>' },
  UModal: { props: ["open"], template: '<section v-if="open"><slot name="content" /><slot name="body" /><slot name="footer" /></section>' },
  UAlert: { props: ['title', 'description'], template: '<div role="alert">{{ title }} {{ description }}<slot name="actions" /></div>' },
  SelectTree: true, SecretInput: { props: ["modelValue"], emits: ["update:modelValue"], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' }, USelectMenu: true,
  UAvatar: true, UBadge: { template: '<span><slot /></span>' },
  UTable: defineComponent({
    props: ['data', 'columns', 'loading'],
    setup: props => () => h('table', props.data.map((row: any) => h('tr', { 'data-user': row.username }, props.columns.map((column: any) => h('td', column.cell ? column.cell({ row: { original: row } }) : row[column.accessorKey]))))),
  }),
  UPagination: defineComponent({
    props: ['page', 'total', 'itemsPerPage', 'disabled'], emits: ['update:page'],
    setup: (props, { emit }) => () => h('nav', [
      h('button', { 'aria-label': t('organization.user.pagination.previous'), disabled: props.disabled || props.page <= 1, onClick: () => emit('update:page', props.page - 1) }, '‹'),
      ...Array.from({ length: Math.max(1, Math.ceil(props.total / props.itemsPerPage)) }, (_, i) => h('button', { disabled: props.disabled, onClick: () => emit('update:page', i + 1) }, String(i + 1))),
      h('button', { 'aria-label': t('organization.user.pagination.next'), disabled: props.disabled || props.page >= Math.max(1, Math.ceil(props.total / props.itemsPerPage)), onClick: () => emit('update:page', props.page + 1) }, '›'),
    ]),
  }),
};
const record = (index: number) => ({
  Member: { uuid: `member-${index}`, username: `person${index}`, display_name: `Person ${index}`, status: 1 },
  User: { uuid: `user-${index}`, display_name: `Person ${index}`, email: `person${index}@example.invalid`, is_root: false },
  DeptIDs: [1, 2],
});
let total = 25;
const response = (params: any) => {
  const count = params.q === 'exact@example.invalid' ? 1 : total;
  const first = (params.page - 1) * params.page_size;
  return { data: {
    items: Array.from({ length: Math.max(0, Math.min(params.page_size, count - first)) }, (_, i) => record(first + i + 1)),
    pagination: { total: count, page: params.page, page_size: params.page_size, pages: Math.ceil(count / params.page_size) },
  } };
};
const render = async (root = false) => {
  const pinia = createPinia();
  const { useUserStore } = await import("~/stores/user");
  useUserStore(pinia).isRoot = root;
  const wrapper = mount(UsersTenantAdmin, { props: { tenantUuid: 'test-tenant' }, global: { plugins: [pinia], stubs, mocks: { $t: t } } });
  await flushPromises();
  return wrapper;
};
const summary = (wrapper: Awaited<ReturnType<typeof render>>) => wrapper.get('[data-testid="user-pagination-summary"]').text();
const go = async (wrapper: Awaited<ReturnType<typeof render>>, page: number) => {
  await wrapper.findAll('nav button').find(button => button.text() === String(page))!.trigger('click');
  await flushPromises();
};

beforeEach(() => {
  vi.clearAllMocks(); total = 25;
  api.list.mockImplementation(async params => response(params));
  api.roles.mockResolvedValue({ data: { role_uuids: ["role-user"] } });
  api.create.mockResolvedValue({ data: { id: 26 } });
});

describe('tenant user pagination', () => {
  it('uses the server total and reaches users beyond the first ten rows', async () => {
    const wrapper = await render();
    expect(summary(wrapper)).toContain('共 25 条');
    expect(summary(wrapper)).toContain('第 1 / 3 页');
    await go(wrapper, 3);
    expect(api.list).toHaveBeenLastCalledWith(expect.objectContaining({ page: 3, page_size: 10 }));
    expect(wrapper.find('[data-user="person25"]').exists()).toBe(true);
    expect(summary(wrapper)).toContain('21-25');
    wrapper.unmount();
  });

  it('resets search to page one and restores pagination when search is cleared', async () => {
    const wrapper = await render(); await go(wrapper, 3);
    await wrapper.get('input').setValue('exact@example.invalid'); await flushPromises();
    expect(api.list).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, q: 'exact@example.invalid' }));
    expect(summary(wrapper)).toContain('共 1 条');
    await wrapper.get('input').setValue(''); await flushPromises();
    expect(summary(wrapper)).toContain('第 1 / 3 页');
    expect(wrapper.findAll('[data-user]')).toHaveLength(10);
    wrapper.unmount();
  });

  it('changes page size from page one while keeping active query and status', async () => {
    const wrapper = await render();
    await wrapper.get('input').setValue('person');
    await wrapper.findAll('select').find(select => select.text().includes(t('organization.user.filter.allStatus')))?.setValue('active');
    await flushPromises(); await go(wrapper, 3);
    await wrapper.get('select[aria-label="每页条数"]').setValue('20'); await flushPromises();
    expect(api.list).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, page_size: 20, q: 'person', status: 1 }));
    expect(wrapper.findAll('[data-user]')).toHaveLength(20);
    expect(summary(wrapper)).toContain('第 1 / 2 页');
    wrapper.unmount();
  });

  it.each([0, 1])('keeps totals and disabled navigation visible for %i results', async count => {
    total = count; const wrapper = await render();
    expect(wrapper.find('[data-testid="user-pagination"]').exists()).toBe(true);
    expect(summary(wrapper)).toContain(count === 0 ? '0-0 / 共 0 条' : '1-1 / 共 1 条');
    expect(wrapper.get('button[aria-label="上一页"]').attributes('disabled')).toBeDefined();
    expect(wrapper.get('button[aria-label="下一页"]').attributes('disabled')).toBeDefined();
    wrapper.unmount();
  });

  it('returns to the last available page after deleting its final row', async () => {
    total = 21; const wrapper = await render(); await go(wrapper, 3);
    vi.spyOn(window, 'confirm').mockReturnValueOnce(true);
    api.remove.mockImplementation(async () => { total = 20; });
    await wrapper.findAll('table button').find(button => button.text() === t('organization.common.delete'))!.trigger('click');
    await flushPromises();
    expect(api.list).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }));
    expect(summary(wrapper)).toContain('第 2 / 2 页');
    expect(wrapper.find('[data-user="person20"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it('ignores an older page response after a newer search completes', async () => {
    const wrapper = await render();
    let finish!: (value: any) => void;
    api.list.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    await wrapper.get('button[aria-label="下一页"]').trigger('click');
    await wrapper.get('input').setValue('exact@example.invalid'); await flushPromises();
    finish(response({ page: 2, page_size: 10 })); await flushPromises();
    expect(summary(wrapper)).toContain('第 1 / 1 页');
    expect(wrapper.findAll('[data-user]')).toHaveLength(1);
    wrapper.unmount();
  });

  it('shows a failed page request and retries without changing the filters', async () => {
    const wrapper = await render();
    const logger = vi.spyOn(console, 'error').mockImplementation(() => {});
    api.list.mockRejectedValueOnce(new Error('缺少底座成员管理权限（corex.iam.member_accounts:read）'));
    await wrapper.get('button[aria-label="下一页"]').trigger('click'); await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain(t('organization.user.listLoadFailed'));
    expect(wrapper.get('[role="alert"]').text()).toContain('corex.iam.member_accounts:read');
    await wrapper.findAll('[role="alert"] button')[0]!.trigger('click'); await flushPromises();
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    expect(summary(wrapper)).toContain('第 2 / 3 页');
    logger.mockRestore(); wrapper.unmount();
  });
});


describe('tenant admin user form', () => {
  const addForm = async (wrapper: Awaited<ReturnType<typeof render>>) => {
    await wrapper.findAll('button').find(button => button.text() === t('organization.user.add'))!.trigger('click');
    await flushPromises();
    await wrapper.get('input[name="px-user-display-name"]').setValue('New Person');
    await wrapper.get('input[name="px-tenant-username"]').setValue('newperson');
    await wrapper.get('input[name="px-user-email"]').setValue('newperson@example.invalid');
    await wrapper.get('input[name="px-user-initial-password"]').setValue('Fixture-secret-123');
    await wrapper.get('input[name="px-user-confirm-password"]').setValue('Fixture-secret-123');
  };
  it('creates through the tenant service and keeps the Root creation API unused', async () => {
    const wrapper = await render(); await addForm(wrapper);
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({ username: 'newperson', role_uuids: ['role-user'] }));
    expect(api.rootCreate).not.toHaveBeenCalled();
    expect(wrapper.find('form').exists()).toBe(false);
    wrapper.unmount();
  });
  it('preserves the Root system user creation path', async () => {
    const wrapper = await render(true); await addForm(wrapper);
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect(api.rootCreate).toHaveBeenCalledWith(expect.objectContaining({ tenant_uuid: 'test-tenant', username: 'newperson' }));
    expect(api.create).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it('keeps entered fields and the modal open after a creation failure', async () => {
    api.create.mockRejectedValueOnce(new Error('fixture creation failed'));
    const wrapper = await render(); await addForm(wrapper);
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect((wrapper.get('input[name="px-tenant-username"]').element as HTMLInputElement).value).toBe('newperson');
    expect(api.rootCreate).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it('edits by member UUID and sends roles in the same update', async () => {
    const wrapper = await render();
    await wrapper.findAll('table button').find(button => button.text() === t('organization.common.edit'))!.trigger('click');
    await flushPromises();
    await wrapper.get('input[name="px-user-display-name"]').setValue('Edited Person');
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect(api.update).toHaveBeenCalledWith('member-1', expect.objectContaining({ display_name: 'Edited Person', role_uuids: ['role-user'] }));
    expect(api.setRoles).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});

describe('formal user department relationships', () => {
  it.each([false, true])('preserves existing departments on a name-only edit (root=%s)', async root => {
    const wrapper = await render(root);
    await wrapper.findAll('table button').find(button => button.text() === t('organization.common.edit'))!.trigger('click');
    await flushPromises();
    const departments = wrapper.getComponent('[data-testid="user-departments"]');
    expect(departments.attributes('modelvalue')).toBe('1,2');
    await wrapper.get('input[name="px-user-display-name"]').setValue('New Name');
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect(api.update).toHaveBeenCalled();
    expect(api.update.mock.calls[0][1]).not.toHaveProperty('dept_ids');
    wrapper.unmount();
  });

  it.each([false, true])('submits changed and explicitly cleared departments (root=%s)', async root => {
    for (const selection of [['2'], []]) {
      api.update.mockClear();
      const wrapper = await render(root);
      await wrapper.findAll('table button').find(button => button.text() === t('organization.common.edit'))!.trigger('click');
      await flushPromises();
      wrapper.getComponent('[data-testid="user-departments"]').vm.$emit('update:modelValue', selection);
      await wrapper.get('form').trigger('submit'); await flushPromises();
      expect(api.update.mock.calls[0][1].dept_ids).toEqual(selection.map(Number));
      wrapper.unmount();
    }
  });

  it('passes department filtering to the server and removes it when reset', async () => {
    const wrapper = await render();
    await go(wrapper, 3);
    wrapper.findComponent({ name: 'SelectTree' }).vm.$emit('update:modelValue', '2');
    await flushPromises();
    expect(api.list).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, dept_id: 2, recursive: true }));
    await wrapper.findAll('button').find(button => button.text() === t('organization.user.filter.reset'))!.trigger('click');
    await flushPromises();
    expect(api.list.mock.calls.at(-1)?.[0]).not.toHaveProperty('dept_id');
    wrapper.unmount();
  });
});
