import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { computed, effectScope, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch, type EffectScope } from 'vue';
import Header from '~/components/layout/Header.vue';
import { useNotifications } from '~/composables/useNotifications';
import { AUTH_SESSION_EVENT, AUTH_SESSION_KEY } from '~/utils/auth-session';

const fixture = vi.hoisted(() => ({ user: null as any, auth: null as any, views: [] as any[] }));
const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), markRead: vi.fn(), delete: vi.fn(), clearAll: vi.fn() }));
vi.mock('~/stores/user', () => ({ useUserStore: () => fixture.user }));
vi.mock('~/composables/useAuth', () => ({ useAuth: () => fixture.auth }));
vi.mock('~/composables/api/services/notificationService', () => ({ useNotificationService: () => api }));

const tenant = '6b5d0240-9920-46da-b707-88200e0f51ea';
const member = '11111111-1111-4111-8111-111111111111';
const other = '22222222-2222-4222-8222-222222222222';
const currentNotice = '00000000-0000-4000-8000-000000000001';
const oldNotice = '00000000-0000-4000-8000-000000000002';

class Socket {
  static OPEN = 1;
  static instances: Socket[] = [];
  readyState = 0;
  onopen?: () => void;
  onclose?: (event: any) => void;
  onmessage?: (event: any) => void;
  send = vi.fn();
  close = vi.fn(() => { this.readyState = 3; });
  constructor(public url: string) { Socket.instances.push(this); }
  open() { this.readyState = 1; this.onopen?.(); }
  receipt(userId = member) {
    this.onmessage?.({ data: JSON.stringify({ type: 'event', topic: '_topic.system.notification', payload: {
      eventType: 'notification_read', payload_version: 1, contract: 'notification_read_receipt.v1',
      tenant_uuid: tenant, userId, notification_uuids: [currentNotice],
    } }) });
  }
}

let wrapper: ReturnType<typeof mount> | undefined;
let scope: EffectScope;
const setSession = (sessionId: string) => {
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify({ version: 1, status: 'active', sessionId, revision: 1,
    accessToken: 'fixture-token', refreshToken: 'fixture-refresh', tokenType: 'Bearer',
    expiresAt: Date.now() + 3600000, scope: 'access', tenantUuid: tenant }));
};
const ready = () => {
  fixture.user.user = { id: 1 };
  fixture.user.currentTenantUuid = tenant;
  fixture.user.currentMemberUuid = member;
  fixture.user.contextSessionStamp = { sessionId: 'session-one', revision: 1 };
};

beforeEach(() => {
  vi.useFakeTimers(); vi.clearAllMocks(); setActivePinia(createPinia());
  Socket.instances = []; fixture.views = []; scope = effectScope();
  fixture.user = reactive({ user: null, currentTenantUuid: null, currentMemberUuid: null, contextSessionStamp: null, fetchUserContext: vi.fn().mockResolvedValue(undefined) });
  fixture.auth = { token: ref('fixture-token'), getToken: () => 'fixture-token', refreshAuth: vi.fn(), logout: vi.fn() };
  setSession('session-one');
  api.list.mockImplementation(async () => ({ data: { items: [currentNotice, oldNotice].map(id => ({ id, userId: fixture.user.currentMemberUuid, isRead: false, createdAt: new Date(), type: 'info', category: 'system' })), pagination: { total: 2 } } }));
  for (const [name, value] of Object.entries({
    process: { ...process, client: true }, WebSocket: Socket,
    computed, ref, watch, onMounted, onBeforeUnmount,
    useRuntimeConfig: () => ({ public: { wsPath: '/api/ws' } }),
    useRoute: () => ({ meta: {}, path: '/notifications' }),
    useLocalePath: () => (path: string) => path,
    useI18n: () => ({ t: (key: string) => key, te: () => false }),
    useToast: () => ({ add: vi.fn() }), useSearch: () => ({ quickSearch: vi.fn() }),
    navigateTo: vi.fn().mockResolvedValue(undefined),
    useState: (_key: string, initial: () => unknown) => ref(initial()),
    useNuxtData: () => ({ data: ref(null) }),
    useNotifications: (initialFilter?: Parameters<typeof useNotifications>[0]) => { const view = useNotifications(initialFilter); fixture.views.push(view); return view; },
  })) vi.stubGlobal(name, value);
});
afterEach(() => { wrapper?.unmount(); wrapper = undefined; scope.stop(); vi.useRealTimers(); vi.unstubAllGlobals(); localStorage.clear(); });

it('runs real Header → WS → shared receipts: 2 to 1, one reconnect read, no polling', async () => {
  wrapper = mount({ ...Header, render: () => null });
  await flushPromises();
  expect(Socket.instances).toHaveLength(0);
  expect(api.list).not.toHaveBeenCalled();
  ready(); await flushPromises();
  const bell = fixture.views[0];
  const list = scope.run(useNotifications)!;
  const detail = scope.run(useNotifications)!;
  await list.fetchNotifications();
  const displayed = list.notifications.value.find(n => n.id === currentNotice)!;
  const first = Socket.instances[0]!; first.open(); await nextTick();
  expect(api.list.mock.calls[0][0].isRead).toBe(false);
  expect(first.send).toHaveBeenCalledWith(JSON.stringify({ type: 'subscribe', topic: '_topic.system.notification' }));
  expect(bell.getStats().unread).toBe(2);
  first.receipt(other); await nextTick();
  expect(bell.getStats().unread).toBe(2);
  first.receipt(); await nextTick();
  expect(bell.getStats().unread).toBe(1);
  expect(bell.totalCount.value).toBe(1);
  expect(bell.notifications.value.map((n: any) => n.id)).toEqual([oldNotice]);
  expect(list.notifications.value.find(n => n.id === oldNotice)?.isRead).toBe(false);
  expect(list.notifications.value.find(n => n.id === currentNotice)?.isRead).toBe(true);
  expect(detail.isNotificationRead(displayed)).toBe(true);
  const reads = api.list.mock.calls.length;
  first.receipt(); await vi.advanceTimersByTimeAsync(60000);
  expect(api.list).toHaveBeenCalledTimes(reads);
  first.onclose?.({ code: 1006 }); await nextTick();
  await vi.advanceTimersByTimeAsync(3000);
  const second = Socket.instances.at(-1)!;
  expect(second).not.toBe(first); second.open(); await flushPromises();
  expect(api.list).toHaveBeenCalledTimes(reads + 1);
  expect(bell.getStats().unread).toBe(1); // stale unread response cannot undo receipt
  wrapper.unmount(); wrapper = undefined;
  expect(second.close).toHaveBeenCalledOnce();
});

it('keeps read history in the notification center and its entry visible when the bell is empty', async () => {
  const history = [currentNotice, oldNotice].map(id => ({ id, title: 'Same order updated', content: 'Read history', userId: member, isRead: true }));
  api.list.mockImplementation(async (params: any) => ({ data: { items: params.isRead === false ? [] : history, pagination: { total: params.isRead === false ? 0 : history.length } } }));
  ready(); wrapper = mount({ ...Header, render: () => null }); await flushPromises();
  const state = wrapper.vm.$.setupState as any;
  expect(state.unreadCount).toBe(0);
  expect(state.notificationItems[0]).toEqual([{ label: 'header.notifications.empty', icon: 'i-heroicons-bell-slash', disabled: true }]);
  const center = state.notificationItems[1][0];
  expect(center.label).toBe('header.notifications.viewAll');
  await center.onSelect();
  expect(navigateTo).toHaveBeenCalledWith('/notifications');
  expect(state.notificationItems.flat().some((n: any) => n.label === 'header.notifications.clear')).toBe(false);
  const historyView = scope.run(useNotifications)!;
  await historyView.fetchNotifications();
  expect(historyView.notifications.value.map(n => n.id)).toEqual([currentNotice, oldNotice]);
  expect(historyView.notifications.value.every(n => n.isRead)).toBe(true);
  expect(api.clearAll).not.toHaveBeenCalled();
});

it('shows five unread items while the bell badge uses the server unread total across pages', async () => {
  const unread = Array.from({ length: 20 }, (_, i) => ({ id: `notice-${i}`, title: `Unread ${i}`, content: '', userId: member, isRead: false }));
  api.list.mockResolvedValueOnce({ data: { items: unread, pagination: { total: 25 } } });
  ready(); wrapper = mount({ ...Header, render: () => null }); await flushPromises();
  const state = wrapper.vm.$.setupState as any;
  expect(api.list.mock.calls[0][0].isRead).toBe(false);
  expect(state.unreadCount).toBe(25);
  expect(state.notificationItems[0].map((n: any) => n.label)).toEqual(unread.slice(0, 5).map(n => n.title));
  expect(state.notificationItems[1][0].badge).toBe('25');
});

it('releases the old socket and waits for the new member context after login changes', async () => {
  ready(); wrapper = mount({ ...Header, render: () => null }); await flushPromises();
  const first = Socket.instances[0]!; first.open(); await nextTick();
  const bell = fixture.views[0];
  setSession('session-two'); window.dispatchEvent(new Event(AUTH_SESSION_EVENT)); await nextTick();
  expect(first.close).toHaveBeenCalledOnce();
  const beforeReady = Socket.instances.length;
  fixture.user.user = { id: 2 }; fixture.user.currentMemberUuid = other;
  fixture.user.contextSessionStamp = { sessionId: 'session-two', revision: 1 };
  await flushPromises();
  expect(Socket.instances.length).toBe(beforeReady + 1);
  const current = Socket.instances.at(-1)!; current.open(); await nextTick();
  first.receipt(); current.receipt(member); await nextTick();
  expect(bell.getStats().unread).toBe(2);
  current.receipt(other); await nextTick();
  expect(bell.getStats().unread).toBe(1);
});
