import { mount, flushPromises } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ApiKeyBatchGrantModal from '~/components/settings/ApiKeyBatchGrantModal.vue';
import zh from '../../../i18n/locales/zh.json';
import streamPermissions from '../../fixtures/iam/llm-stream-permissions.json';
const mocks = vi.hoisted(() => ({ save: vi.fn() }));
vi.mock('~/composables/api/services/integrationGatewayApiKeyService', () => ({
  useIntegrationGatewayApiKeyService: () => ({
    batchAppendExplicitGrants: mocks.save,
  }),
}));
vi.mock('#imports', () => ({
  useI18n: () => ({
    t: (key: string, vars: Record<string, unknown> = {}) => {
      let text = key.split('.').reduce((v: any, k) => v?.[k], zh) ?? key;
      for (const [k, v] of Object.entries(vars))
        text = text.replaceAll(`{${k}}`, String(v));
      return text;
    },
  }),
}));
const stubs = {
  UModal: {
    props: ['open', 'title', 'description'],
    template:
      '<section v-if="open" role="dialog"><h2>{{title}}</h2><p>{{description}}</p><slot name="body"/><slot name="footer"/></section>',
  },
  UButton: {
    props: ['disabled', 'loading'],
    template: '<button :disabled="disabled || loading"><slot/></button>',
  },
  UAlert: {
    props: ['title', 'description'],
    template: '<div>{{title}} {{description}}</div>',
  },
  UInput: {
    props: ['modelValue', 'disabled', 'placeholder'],
    emits: ['update:modelValue'],
    template:
      '<input :disabled="disabled" :placeholder="placeholder" :value="modelValue" @input="$emit(\'update:modelValue\',$event.target.value)"/>',
  },
  UCheckbox: {
    props: ['modelValue', 'disabled'],
    emits: ['update:modelValue'],
    template:
      '<input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\',$event.target.checked)"/>',
  },
};
const key = (id: string, status = 'active') => ({
  key_id: id,
  name: id,
  plugin_ids: ['com.example.' + id],
  profile_id: 1,
  tenant_uuid: 'tenant',
  binding_mode: 'key',
  status,
  key_prefix: 'prefix',
  created_at: '',
  updated_at: '',
});
const capability = {
  id: 9,
  module: 'ecommerce',
  resource: 'plans',
  action: 'read',
  status: 'active',
  meta: {
    label: '电商套餐读取',
    api_key_explicit: true,
    capability_id: 'com.example.plans.read',
    api_key: { plugin_id: 'com.example.ecommerce', scope: '_scope.plans.read' },
  },
};
function result(committed = false) {
  return {
    committed,
    key_count: 2,
    added_count: 2,
    plan_hash: 'a'.repeat(64),
    keys: ['workspace', 'crm'].map((id) => ({
      key_id: id,
      name: id,
      owner_plugin_ids: ['com.example.' + id],
      added_count: 1,
      existing_count: 0,
    })),
    permissions: [
      {
        permission_id: 9,
        name: '电商套餐读取',
        capability_id: 'com.example.plans.read',
        action: 'read',
        resource_type: 'capability',
        resource_pattern: 'plans',
      },
    ],
  };
}
function setup() {
  return mount(ApiKeyBatchGrantModal, {
    props: {
      open: true,
      tenantUuid: 'tenant',
      profileId: 1,
      profileName: 'Shared',
      keys: [key('workspace'), key('crm'), key('disabled', 'revoked')] as any,
      permissions: [
        capability,
        {
          ...capability,
          id: 10,
          meta: { ...capability.meta, api_key_explicit: false },
        },
      ],
    },
    global: { stubs },
  });
}
async function click(wrapper: ReturnType<typeof setup>, name: string) {
  const button = wrapper
    .findAll('button')
    .find((button) => button.text() === name);
  expect(button, name).toBeTruthy();
  await button!.trigger('click');
  await flushPromises();
}
async function select(wrapper: ReturnType<typeof setup>) {
  await click(wrapper, '选择当前列表');
  await click(wrapper, '下一步');
  expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(1);
  await wrapper.get('input[type="checkbox"]').setValue(true);
}
beforeEach(() => mocks.save.mockReset());
describe('batch independent grants', () => {
  it('keeps the same-name HTTP grant out of service selection and appends only the requested Key', async () => {
    const wrapper = setup();
    await wrapper.setProps({ open: false });
    await wrapper.setProps({
      open: true,
      permissions: streamPermissions as any,
      initialKeyIds: ['workspace'],
      initialPermissionIds: [streamPermissions[0].id],
    });
    await click(wrapper, zh.integrationApiKeyBatch.next);
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(1);
    expect(wrapper.text()).toContain(zh.integrationApiKeyAccess.service);
    expect(wrapper.text()).not.toContain(zh.integrationApiKeyAccess.http);
    expect(wrapper.text()).toContain(streamPermissions[1].meta.core_internal_endpoint);
    expect((wrapper.get('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false);
    await wrapper.get('input[type="checkbox"]').setValue(true);
    const plan = {
      ...result(), key_count: 1, added_count: 1,
      keys: result().keys.slice(0, 1),
      permissions: [{
        permission_id: streamPermissions[1].id,
        name: streamPermissions[1].meta.label,
        capability_id: streamPermissions[1].meta.capability_id,
        ...streamPermissions[1].meta.api_key,
      }],
    };
    mocks.save.mockResolvedValueOnce({ data: plan }).mockResolvedValueOnce({ data: { ...plan, committed: true } });
    await click(wrapper, zh.integrationApiKeyBatch.preview);
    await click(wrapper, zh.integrationApiKeyBatch.commit);
    expect(mocks.save).toHaveBeenNthCalledWith(2, {
      key_ids: ['workspace'], permission_ids: [streamPermissions[1].id],
      dry_run: false, expected_plan_hash: plan.plan_hash,
    });
    wrapper.unmount();
  });

  it('prefills only current Key and explicit capability IDs when opened from a conflict, then keeps them editable', async () => {
    const wrapper = setup();
    await wrapper.setProps({ open: false });
    await wrapper.setProps({
      open: true,
      initialKeyIds: ['workspace', 'disabled', 'foreign'],
      initialPermissionIds: [9, 10, 99],
    });
    expect(wrapper.text()).toContain('已选 1 个 Key');
    await click(wrapper, '下一步');
    expect(
      (wrapper.get('input[type="checkbox"]').element as HTMLInputElement)
        .checked,
    ).toBe(true);
    expect(mocks.save).not.toHaveBeenCalled();
    await wrapper.get('input[type="checkbox"]').setValue(false);
    expect(wrapper.text()).toContain('已选 0 项能力');
    wrapper.unmount();
  });

  it('previews before saving, submits only selected IDs and keeps the review visible on success', async () => {
    mocks.save
      .mockResolvedValueOnce({ data: result() })
      .mockResolvedValueOnce({ data: result(true) });
    const wrapper = setup();
    await select(wrapper);
    expect(mocks.save).not.toHaveBeenCalled();
    await click(wrapper, '预览授权');
    expect(mocks.save).toHaveBeenNthCalledWith(1, {
      key_ids: ['workspace', 'crm'],
      permission_ids: [9],
      dry_run: true,
    });
    expect(wrapper.text()).toContain('2 个 Key × 1 项能力');
    await click(wrapper, '确认保存整批授权');
    expect(mocks.save).toHaveBeenNthCalledWith(2, {
      key_ids: ['workspace', 'crm'],
      permission_ids: [9],
      dry_run: false,
      expected_plan_hash: 'a'.repeat(64),
    });
    expect(wrapper.text()).toContain('已为 2 个 Key 保存独立授权');
    expect(wrapper.emitted('saved')).toHaveLength(1);
    wrapper.unmount();
  });
  it('retains selections and identifies failed permissions; repeated preview needs no reselection', async () => {
    mocks.save
      .mockRejectedValueOnce({
        status: 409,
        data: {
          error_code: 'CAPABILITY_TENANT_NOT_REGISTERED',
          details: { failed_permission_name: '电商套餐读取' },
        },
      })
      .mockResolvedValueOnce({ data: result() });
    const wrapper = setup();
    await select(wrapper);
    await click(wrapper, '预览授权');
    expect(wrapper.text()).toContain('电商套餐读取：当前租户尚未注册该能力');
    expect(
      (wrapper.get('input[type="checkbox"]').element as HTMLInputElement)
        .checked,
    ).toBe(true);
    await click(wrapper, '预览授权');
    expect(wrapper.text()).toContain('2 个 Key × 1 项能力');
    wrapper.unmount();
  });
  it('treats interrupted commit as unknown and requires a new preview without automatic retry', async () => {
    mocks.save
      .mockResolvedValueOnce({ data: result() })
      .mockRejectedValueOnce(new Error('network interrupted'))
      .mockResolvedValueOnce({ data: { ...result(), added_count: 0 } });
    const wrapper = setup();
    await select(wrapper);
    await click(wrapper, '预览授权');
    await click(wrapper, '确认保存整批授权');
    expect(wrapper.text()).toContain('保存结果尚未确认');
    expect(mocks.save).toHaveBeenCalledTimes(2);
    expect(
      wrapper.findAll('button').some((b) => b.text() === '确认保存整批授权'),
    ).toBe(false);
    await click(wrapper, '重新预览');
    expect(mocks.save).toHaveBeenCalledTimes(3);
    expect(mocks.save.mock.calls[2][0].dry_run).toBe(true);
    expect(wrapper.emitted('saved')).toBeUndefined();
    wrapper.unmount();
  });
  it('ignores a late response after tenant/profile switch and closes the old dialog', async () => {
    let finish!: (v: any) => void;
    mocks.save.mockImplementationOnce(
      () => new Promise((resolve) => (finish = resolve)),
    );
    const wrapper = setup();
    await select(wrapper);
    await click(wrapper, '预览授权');
    expect(
      wrapper
        .findAll('input')
        .every((input) => input.attributes('disabled') !== undefined),
    ).toBe(true);
    await wrapper.setProps({ profileId: 2 });
    finish({ data: result() });
    await flushPromises();
    expect(wrapper.emitted('update:open')?.at(-1)).toEqual([false]);
    expect(wrapper.text()).not.toContain('2 个 Key × 1 项能力');
    expect(wrapper.emitted('saved')).toBeUndefined();
    wrapper.unmount();
  });
  it('rejects partial or duplicate response rows and never displays success', async () => {
    mocks.save.mockResolvedValueOnce({
      data: { ...result(), keys: [result().keys[0], result().keys[0]] },
    });
    const wrapper = setup();
    await select(wrapper);
    await click(wrapper, '预览授权');
    expect(wrapper.text()).toContain('批量授权未完成');
    expect(wrapper.text()).not.toContain('确认保存整批授权');
    wrapper.unmount();
  });
});
