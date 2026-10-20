import { createPinia, setActivePinia } from 'pinia';
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils';
import { computed, nextTick, reactive, ref, watch } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import NotificationPage from '~/pages/notifications/index.vue';
import { useNotifications } from '~/composables/useNotifications';
import { AUTH_SESSION_KEY } from '~/utils/auth-session';

const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), markRead: vi.fn(), delete: vi.fn(), clearAll: vi.fn() }));
const fixture = vi.hoisted(() => ({ user: null as any }));
vi.mock('~/stores/user', () => ({ useUserStore: () => fixture.user }));
vi.mock('~/composables/api/services/notificationService', () => ({ useNotificationService: () => api }));
enableAutoUnmount(afterEach);
const tenant = '11111111-1111-4111-8111-111111111111';
const member = '22222222-2222-4222-8222-222222222222';
const id = '33333333-3333-4333-8333-333333333333';
const route = reactive({ path: '/notifications', query: {} as Record<string, string> });
const width = ref(1440);
const record = (title = '持久化备份结果') => ({ id, userId: fixture.user.currentMemberUuid, title, content: '备份任务 #803 已完成', createdAt: new Date().toISOString(), type: 'success', category: 'system', isRead: false });
const toast = vi.fn();
const deferred = () => { let resolve!: (input: any) => void; const promise = new Promise<any>(r => { resolve = r; }); return {promise, resolve}; };
const stubs = {
  UIcon: true, UBadge: { template: '<span><slot/></span>' },
  UButton: { props: ['disabled'], template: '<button :disabled="disabled"><slot/></button>' },
  USelect: { props: ['items', 'modelValue'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :value="item.value">{{item.label}}</option></select>' },
  UCheckbox: true, UPagination: true,
};
const page = () => mount(NotificationPage, {global: {stubs}});
beforeEach(() => {
  vi.resetAllMocks(); setActivePinia(createPinia());
  vi.stubGlobal('process', { ...process, client: true, emit: process.emit.bind(process) });
  fixture.user = reactive({ user: {id: 1}, currentTenantUuid: tenant, currentMemberUuid: member, contextSessionStamp: {sessionId: 'notification-session', revision: 1} });
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify({ version: 1, status: 'active', sessionId: 'notification-session', revision: 1, accessToken: 'fixture', refreshToken: 'fixture', tokenType: 'Bearer', expiresAt: Date.now() + 60000, scope: 'access', tenantUuid: tenant }));
  api.list.mockResolvedValue({data: {items: [], pagination: {total: 0}}});
  api.get.mockImplementation(async () => ({data: record()}));
  api.markRead.mockResolvedValue({});
  route.query = {id}; width.value = 1440;
  for (const [key, value] of Object.entries({ computed, ref, watch, useNotifications,
    definePageMeta: vi.fn(), useHead: vi.fn(), useRoute: () => route,
    useWindowSize: () => ({width}), useI18n: () => ({t: (key: string) => key}), useToast: () => ({add: toast}),
    navigateTo: vi.fn(async (to) => { route.query = to.query || {}; }),
  })) vi.stubGlobal(key, value);
});
afterEach(() => { vi.unstubAllGlobals(); localStorage.clear(); });

describe('notification center direct links', () => {
  it.each([1440, 390])('loads the URL record outside the first page and marks it read after display at width %i', async size => {
    width.value = size;
    const wrapper = page(); await flushPromises();
    expect(api.get).toHaveBeenCalledWith(id);
    expect(wrapper.text()).toContain('持久化备份结果');
    expect(api.markRead).toHaveBeenCalledOnce();
    expect(api.markRead).toHaveBeenCalledWith(id);
    expect(wrapper.find('[data-testid="notification-detail-state"]').exists()).toBe(false);
  });
  it('waits for member context, shows a missing notification, and supports retry and return', async () => {
    fixture.user.contextSessionStamp = null;
    const wrapper = page(); await flushPromises();
    expect(api.get).not.toHaveBeenCalled();
    expect(api.list).not.toHaveBeenCalled();
    api.get.mockRejectedValueOnce({statusCode: 404});
    fixture.user.contextSessionStamp = {sessionId: 'notification-session', revision: 1};
    await flushPromises();
    expect(wrapper.get('[data-testid="notification-detail-state"]').text()).toContain('header.notifications.unavailable');
    expect(api.markRead).not.toHaveBeenCalled();
    await wrapper.findAll('button').find(b => b.text() === 'header.notifications.retry')!.trigger('click');
    await flushPromises();
    expect(wrapper.text()).toContain('持久化备份结果');
    await wrapper.findAll('button').find(b => b.text() === '返回')!.trigger('click');
    await flushPromises();
    expect(route.query.id).toBeUndefined();
    expect(wrapper.text()).not.toContain('持久化备份结果');
  });
  it('discards late results after a route or member change', async () => {
    const old = deferred(); api.get.mockReturnValueOnce(old.promise);
    const wrapper = page(); await flushPromises();
    const nextID = '44444444-4444-4444-8444-444444444444';
    api.get.mockResolvedValueOnce({data: {...record('新通知'), id: nextID}});
    route.query = {id: nextID};
    await flushPromises();
    old.resolve({data: record('旧通知')}); await flushPromises();
    expect(wrapper.text()).toContain('新通知'); expect(wrapper.text()).not.toContain('旧通知');
    const previousMember = fixture.user.currentMemberUuid;
    api.get.mockResolvedValueOnce({data: {...record('其他成员内容'), userId: previousMember}});
    fixture.user.currentMemberUuid = '55555555-5555-4555-8555-555555555555';
    await flushPromises();
    expect(wrapper.text()).not.toContain('新通知'); expect(wrapper.text()).not.toContain('其他成员内容');
    expect(wrapper.get('[data-testid="notification-detail-state"]').text()).toContain('header.notifications.unavailable');
  });
  it('renders content even if marking read fails', async () => {
    api.markRead.mockRejectedValue(new Error('network unavailable'));
    const wrapper = page(); await flushPromises();
    expect(wrapper.text()).toContain('持久化备份结果');
    expect(toast).toHaveBeenCalledWith({title: 'header.notifications.readError', color: 'error'});
  });
  it('keeps all read states when changing another filter', async () => {
    const wrapper = page(); await flushPromises();
    await wrapper.findAll('select')[0]!.setValue('system'); await flushPromises();
    expect(api.list).toHaveBeenLastCalledWith(expect.objectContaining({category: 'system', isRead: undefined}));
  });
  it('preserves list filters through narrow detail navigation and screen resizing', async () => {
    width.value = 390; route.query = {};
    const wrapper = page(); await flushPromises();
    await wrapper.findAll('select')[0]!.setValue('system'); await flushPromises();
    route.query = {id}; await flushPromises();
    width.value = 1440; await nextTick();
    expect((wrapper.findAll('select')[0]!.element as HTMLSelectElement).value).toBe('system');
    width.value = 390; await nextTick();
    await wrapper.findAll('button').find(b => b.text() === '返回')!.trigger('click'); await flushPromises();
    expect((wrapper.findAll('select')[0]!.element as HTMLSelectElement).value).toBe('system');
  });
  it('does not manufacture an ID for transient task events', async () => {
    let bell!: ReturnType<typeof useNotifications>;
    const wrapper = mount({setup() { bell = useNotifications(); return () => null; }});
    await bell.addNotification({title: '备份任务更新'});
    await bell.addNotification({id: '   ', title: '恢复任务更新'});
    expect(bell.notifications.value).toHaveLength(0);
    expect(bell.totalCount.value).toBe(0);
    await bell.addNotification(record() as any);
    expect(bell.notifications.value[0]?.id).toBe(id);
    wrapper.unmount();
  });
});
