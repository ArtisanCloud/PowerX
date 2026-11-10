import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh.json';
import Panel from '~/components/monitor/storage/DatabaseRetentionPanel.vue';

const api = vi.hoisted(() => ({ databaseStatus: vi.fn(), saveDatabasePolicy: vi.fn(), previewDatabase: vi.fn(), runDatabase: vi.fn(),adoptDatabasePolicy:vi.fn(),changeDatabasePolicy:vi.fn(),databasePolicyRequest:vi.fn(),databaseRunRequest:vi.fn() }));
vi.mock('~/composables/api/services/storageRuntimeService', () => ({ useStorageRuntimeService: () => api }));
const stubs = {
  UAlert: { props: ['title', 'description'], template: '<section><p>{{title}} {{description}}</p><slot name="actions"/></section>' },
  UCard: { template: '<div><slot/></div>' },
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' },
  UModal: { props: ['open'], template: '<section v-if="open"><slot name="body"/><slot name="footer"/></section>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot/></label>' },
  UInput: { props: ['modelValue', 'disabled'], emits: ['update:modelValue'], template: '<input :disabled="disabled" :value="modelValue" @input="$emit(\'update:modelValue\', Number($event.target.value))"/>' },
  UCheckbox: { props: ['modelValue', 'label', 'disabled'], emits: ['update:modelValue'], template: '<label>{{label}}<input type="checkbox" :disabled="disabled" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)"/></label>' },
};
let status: any;
let wrapper: ReturnType<typeof mount>;
const button = (label: string) => wrapper.findAll('button').find(b => b.text() === label)!;
beforeEach(() => {
  vi.resetAllMocks();
  status = { configuration:{authority:'database',state:'applied',ready:true,execution_ready:true,desired_revision:1,applied_revision:1,snapshot_revision:1,legacy_digest:'a'.repeat(64),effective_capacity:null},state_configured: false, enforcement_allowed: false, migration_ready: false, worker_enabled: false, ready: false, problem: 'database_retention_runtime_not_configured', migration_problem: 'database_retention_migration_required', relation_bytes: 1024, runs: [], policy: { revision: 0, enabled: false, reject_new_when_full: false, adopt_legacy: false, success_days: 30, failure_days: 90, pending_hours: 24, quarantine_days: 30, batch_limit: 200, max_bytes: 8 * 1024 ** 3 } };
  api.databaseStatus.mockImplementation(async () => structuredClone(status));
  wrapper = mount(Panel, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs } });
});
afterEach(() => { wrapper.unmount(); vi.useRealTimers(); });

it('shows deployment and schema prerequisites independently without suggesting cleanup is enabled', async () => {
  await flushPromises();
  expect(wrapper.text()).toContain('治理运行环境未配置');
  expect(wrapper.text()).toContain('数据库结构尚未就绪');
  await button('查看准备步骤').trigger('click');
  expect(wrapper.text()).toContain('当前为观察模式');
  expect(wrapper.text()).toContain('尚缺期限治理字段');
  expect(wrapper.text()).toContain('调用容量保护：未开启');
  expect(button('配置策略').attributes('disabled')).toBeDefined();
  expect(button('检查本批候选').attributes('disabled')).toBeDefined();
  expect(api.saveDatabasePolicy).not.toHaveBeenCalled();
  expect(api.runDatabase).not.toHaveBeenCalled();
});

it('refreshes readiness on manual action and push revision, clears stale errors and never polls', async () => {
  await flushPromises();
  api.databaseStatus.mockRejectedValueOnce(new Error('connection lost'));
  await button('刷新观测').trigger('click'); await flushPromises();
  expect(wrapper.text()).toContain('connection lost');
  status = { ...status, state_configured: true, enforcement_allowed: true, migration_ready: true, worker_enabled: true, ready: true, problem: undefined, migration_problem: undefined };
  await wrapper.setProps({ revision: 2 }); await flushPromises();
  expect(wrapper.text()).not.toContain('connection lost');
  expect(button('配置策略').attributes('disabled')).toBeUndefined();
  expect(button('检查本批候选').attributes('disabled')).toBeUndefined();
  vi.useFakeTimers(); const count = api.databaseStatus.mock.calls.length;
  await vi.advanceTimersByTimeAsync(3600000);
  expect(api.databaseStatus).toHaveBeenCalledTimes(count);
  const signal = api.databaseStatus.mock.calls.at(-1)![0];
  wrapper.unmount(); expect(signal.aborted).toBe(true);
});

it('preserves open form edits during push refresh and disables activation when deployment is blocked', async () => {
  await flushPromises();
  status.state_configured = true; status.problem = 'database_retention_enforcement_not_allowed';
  await button('刷新观测').trigger('click'); await flushPromises();
  await button('配置策略').trigger('click');
  const first = wrapper.findAll('input').find(x => x.attributes('type') !== 'checkbox')!;
  await first.setValue('14');
  await wrapper.setProps({ revision: 3 }); await flushPromises();
  expect((first.element as HTMLInputElement).value).toBe('14');
  const activation = wrapper.findAll('input[type="checkbox"]');
  expect(activation[1]!.attributes('disabled')).toBeDefined();
  expect(wrapper.text()).toContain('保存期限后自动清理保持暂停');
  expect(wrapper.text()).not.toContain('独立开启两张调用审计表的期限治理');
  expect(api.saveDatabasePolicy).not.toHaveBeenCalled();
});
