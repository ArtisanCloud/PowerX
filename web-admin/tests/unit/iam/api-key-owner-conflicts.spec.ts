import { mount } from '@vue/test-utils';
import { ref } from 'vue';
import { describe, expect, it, vi } from 'vitest';
import { parseApiKeyOwnerConflicts } from '~/utils/apiKeyOwnerConflicts';
import ApiKeyOwnerConflictNotice from '~/components/settings/ApiKeyOwnerConflictNotice.vue';
import zh from '../../../i18n/locales/zh.json';

const t = (key: string, vars: Record<string, unknown> = {}) => {
  let value =
    key.split('.').reduce((part: any, name) => part?.[name], zh) ?? key;
  for (const [name, replacement] of Object.entries(vars))
    value = value.replaceAll(`{${name}}`, String(replacement));
  return value;
};
vi.mock('#imports', () => ({
  useI18n: () => ({
    t: (...args: Parameters<typeof t>) => t(...args),
    locale: ref('zh'),
  }),
}));
const item = (name: string, id: number, mode = 'key') => ({
  key_id: `key-${id}`,
  key_name: name,
  binding_mode: mode,
  owner_plugin_ids: ['com.powerx.plugins.workspacex'],
  permission_owner_plugin_ids:
    mode === 'key' ? ['com.powerx.plugins.workspacex'] : [],
  permission_id: 3,
  permission_name: '电商套餐读取',
  capability_id: 'com.powerx.plugins.ecommerce.plans.read',
  required_plugin_id: 'com.powerx.plugins.ecommerce',
  scope: '_scope.ecommerce.plans.read',
  action: 'read',
  resource_type: 'capability',
  resource_pattern: 'plans',
});
const envelope = () => ({
  error_code: 'API_KEY_PLUGIN_OWNER_CONFLICT',
  message: '配置冲突',
  request_id: 'request-id',
  details: {
    profile_id: 1,
    profile_name: 'Shared Profile',
    conflict_count: 2,
    conflicting_key_count: 2,
    conflicts: [
      item('WorkspaceX Key', 1),
      item('CRM Key', 2, 'legacy_permissions'),
    ],
    truncated: false,
    trace_id: 'conflict-trace',
  },
});
const stubs = {
  UAlert: {
    props: ['title', 'description'],
    template:
      '<div role="alert">{{ title }} {{ description }}<slot name="actions" /></div>',
  },
  UButton: { template: '<button><slot /></button>' },
  UModal: {
    props: ['open', 'title', 'description'],
    template:
      '<section v-if="open" role="dialog"><h2>{{title}}</h2><p>{{description}}</p><slot name="body" /><slot name="footer" /></section>',
  },
};

describe('API Key owner conflict diagnostics', () => {
  it('parses the API client envelope, ofetch response and structured details without using the error message', () => {
    const data = envelope();
    for (const error of [
      { data },
      { response: { _data: data } },
      { errorCode: data.error_code, details: data.details },
    ]) {
      const report = parseApiKeyOwnerConflicts(error)!;
      expect(report.conflict_count).toBe(2);
      expect(report.conflicts.map((c) => c.key_name)).toEqual([
        'WorkspaceX Key',
        'CRM Key',
      ]);
      expect(report.trace_id).toBe('conflict-trace');
    }
  });
  it.each([
    { message: 'network error' },
    { error_code: 'OTHER', details: envelope().details },
    { error_code: 'API_KEY_PLUGIN_OWNER_CONFLICT' },
    {
      error_code: 'API_KEY_PLUGIN_OWNER_CONFLICT',
      details: { conflicts: [null] },
    },
  ])(
    'retains generic error handling for unsupported or incomplete errors',
    (error) => {
      expect(parseApiKeyOwnerConflicts(error)).toBeNull();
    },
  );
  it('keeps an explicit empty owner list empty and exposes truthful truncated counts', () => {
    const data = envelope();
    data.details.conflict_count = 300;
    data.details.truncated = true;
    data.details.conflicts[0].owner_plugin_ids = [];
    const report = parseApiKeyOwnerConflicts({ data })!;
    expect(report.conflicts[0].owner_plugin_ids).toEqual([]);
    expect(report.conflict_count).toBe(300);
    expect(report.truncated).toBe(true);
  });
  it('shows every affected key, permission, required owner and the legacy permission-specific scope', async () => {
    const report = parseApiKeyOwnerConflicts({ data: envelope() })!;
    const wrapper = mount(ApiKeyOwnerConflictNotice, {
      props: { report, operation: 'profile' },
      global: { stubs },
    });
    expect(wrapper.get('[role="alert"]').text()).toContain('发现 2 项冲突');
    expect(wrapper.text()).toContain('当前勾选已保留');
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    await wrapper.get('button').trigger('click');
    const dialog = wrapper.get('[role="dialog"]');
    expect(dialog.text()).toContain('WorkspaceX Key');
    expect(dialog.text()).toContain('CRM Key');
    expect(dialog.text()).toContain('电商套餐读取');
    expect(dialog.text()).toContain('com.powerx.plugins.ecommerce');
    expect(dialog.text()).toContain('com.powerx.plugins.workspacex');
    expect(dialog.text()).toContain('此权限原有插件范围');
    expect(dialog.text()).toContain('未绑定');
    expect(dialog.text()).toContain('清空绑定仍会冲突');
    expect(dialog.text()).toContain('conflict-trace');
    await wrapper.setProps({ report: { ...report, profile_id: 2 } });
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    wrapper.unmount();
  });
  it('keeps owner inputs unchanged and renders names as text', async () => {
    const report = parseApiKeyOwnerConflicts({ data: envelope() })!;
    report.conflicts[0].key_name = '<script>unsafe</script>';
    const wrapper = mount(ApiKeyOwnerConflictNotice, {
      props: { report, operation: 'owners' },
      global: { stubs },
    });
    expect(wrapper.text()).toContain('输入内容已保留');
    await wrapper.get('button').trigger('click');
    expect(wrapper.text()).toContain('本次提交的插件范围');
    expect(wrapper.text()).toContain('<script>unsafe</script>');
    expect(wrapper.find('script').exists()).toBe(false);
    wrapper.unmount();
  });
});
