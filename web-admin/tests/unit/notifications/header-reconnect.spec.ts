import { flushPromises, mount } from '@vue/test-utils';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import Header from '~/components/layout/Header.vue';
import NotificationPage from '~/pages/notifications/index.vue';
import { AUTH_SESSION_KEY } from '~/utils/auth-session';

const fixture = vi.hoisted(() => ({ user: null as any, bus: null as any }));
vi.mock('~/stores/user', () => ({ useUserStore: () => fixture.user }));
vi.mock('~/composables/useAuth', () => ({ useAuth: () => ({ logout: vi.fn() }) }));
vi.mock('~/composables/useWSBus', () => ({ useWSBus: () => fixture.bus }));
const fetchNotifications = vi.fn();
const identityScope = ref('tenant:member:actor:session-one');
let wrapper: ReturnType<typeof mount> | undefined;

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('process', { ...process, client: true });
  fixture.user = reactive({ currentTenantUuid: '11111111-1111-4111-8111-111111111111', currentMemberUuid: '22222222-2222-4222-8222-222222222222', contextSessionStamp: { sessionId: 'session-one', revision: 1 }, user: { id: 1 }, fetchUserContext: vi.fn().mockResolvedValue(undefined) });
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify({ version: 1, status: 'active', sessionId: 'session-one', revision: 1, accessToken: 'fixture', refreshToken: 'fixture', tokenType: 'Bearer', expiresAt: Date.now() + 60000, scope: 'access', tenantUuid: fixture.user.currentTenantUuid }));
  fixture.bus = { connected: ref(false), connect: vi.fn(), subscribe: vi.fn(() => vi.fn()) };
  identityScope.value = 'tenant:member:actor:session-one';
  fetchNotifications.mockResolvedValue(undefined);
  for (const [name, value] of Object.entries({
    computed, ref, watch, onMounted, onBeforeUnmount,
    useRoute: () => ({ meta: {}, path: '/notifications' }),
    navigateTo: vi.fn().mockResolvedValue(undefined),
    useLocalePath: () => (path: string) => path,
    useI18n: () => ({ t: (key: string) => key, te: () => false }),
    useToast: () => ({ add: vi.fn() }),
    useState: (_key: string, initial: () => unknown) => ref(initial()),
    useNuxtData: () => ({ data: ref(null) }),
    useSearch: () => ({ quickSearch: vi.fn() }),
    useNotifications: () => ({ identityScope, identityReady: ref(true), getNotification: vi.fn(), notifications: ref([]), totalCount: ref(0), getStats: () => ({ unread: 0 }), fetchNotifications, addNotification: vi.fn(), clearAllNotifications: vi.fn(), openNotification: vi.fn(), applyReadReceipt: vi.fn() }),
  })) vi.stubGlobal(name, value);
});

afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals(); localStorage.clear(); });

it('loads once per identity, refreshes once per reconnect and releases its subscription', async () => {
  // 保留真实 Header setup，仅省略与通知无关的菜单渲染。
  wrapper = mount({ ...Header, render: () => null });
  await flushPromises();
  expect(fetchNotifications).toHaveBeenCalledTimes(1);
  fixture.bus.connected.value = true;
  await nextTick();
  expect(fetchNotifications).toHaveBeenCalledTimes(1);
  fixture.bus.connected.value = false;
  await nextTick();
  fixture.bus.connected.value = true;
  await nextTick();
  expect(fetchNotifications).toHaveBeenCalledTimes(2);

  fixture.bus.connected.value = false; // the transport drops the old login socket
  identityScope.value = 'tenant:member:actor:session-two';
  await nextTick();
  expect(fetchNotifications).toHaveBeenCalledTimes(3);
  fixture.bus.connected.value = true;
  await nextTick();
  expect(fetchNotifications).toHaveBeenCalledTimes(3);
  const unsubscribe = fixture.bus.subscribe.mock.results.at(-1)!.value;
  wrapper.unmount(); wrapper = undefined;
  expect(unsubscribe).toHaveBeenCalledOnce();
  fixture.bus.connected.value = false;
  await nextTick();
  fixture.bus.connected.value = true;
  await nextTick();
  expect(fetchNotifications).toHaveBeenCalledTimes(3);
});

it('waits for the authenticated member context and releases the previous session subscription', async () => {
  fixture.user.contextSessionStamp = null;
  wrapper = mount({ ...Header, render: () => null });
  await flushPromises();
  expect(fixture.bus.subscribe).not.toHaveBeenCalled();
  expect(fetchNotifications).not.toHaveBeenCalled();
  fixture.user.contextSessionStamp = { sessionId: 'session-one', revision: 1 };
  await nextTick();
  expect(fixture.bus.subscribe).toHaveBeenCalledOnce();
  expect(fetchNotifications).toHaveBeenCalledOnce();
  const first = fixture.bus.subscribe.mock.results[0].value;
  const session = JSON.parse(localStorage.getItem(AUTH_SESSION_KEY)!);
  session.sessionId = 'session-two';
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify(session));
  identityScope.value = 'tenant:member:actor:session-two';
  await nextTick();
  expect(first).toHaveBeenCalledOnce();
  expect(fixture.bus.subscribe).toHaveBeenCalledOnce();
  fixture.user.contextSessionStamp = { sessionId: 'session-two', revision: 1 };
  await nextTick();
  expect(fixture.bus.subscribe).toHaveBeenCalledTimes(2);
  expect(fetchNotifications).toHaveBeenCalledTimes(2);
});

it('recovers once after reconnect when the shared socket was already connected at mount', async () => {
  fixture.bus.connected.value = true;
  wrapper = mount({ ...Header, render: () => null });
  await flushPromises();
  expect(fetchNotifications).toHaveBeenCalledTimes(1);
  fixture.bus.connected.value = false;
  await nextTick();
  fixture.bus.connected.value = true;
  await nextTick();
  expect(fetchNotifications).toHaveBeenCalledTimes(2);
});

it('tracks an already open shared socket when member context becomes ready later', async () => {
  fixture.user.contextSessionStamp = null;
  fixture.bus.connected.value = true;
  wrapper = mount({ ...Header, render: () => null });
  await flushPromises();
  fixture.user.contextSessionStamp = { sessionId: 'session-one', revision: 1 };
  await nextTick();
  expect(fetchNotifications).toHaveBeenCalledOnce();
  fixture.bus.connected.value = false; await nextTick();
  fixture.bus.connected.value = true; await nextTick();
  expect(fetchNotifications).toHaveBeenCalledTimes(2);
});

it.each([1440, 390])('clears selected notification after a session change at width %i', async width => {
  vi.stubGlobal('definePageMeta', vi.fn());
  vi.stubGlobal('useHead', vi.fn());
  vi.stubGlobal('useWindowSize', () => ({ width: ref(width) }));
  wrapper = mount(NotificationPage, {
    global: { stubs: {
      NotificationList: { emits: ['select'], template: '<button data-test="select" @click="$emit(\'select\', {id: \'fixture_notice\'})"></button>' },
      NotificationDetail: { props: ['notification'], template: '<div data-test="detail">{{ notification?.id }}</div>' },
    } },
  });
  await wrapper.get('[data-test="select"]').trigger('click');
  expect(wrapper.get('[data-test="detail"]').text()).toBe('fixture_notice');
  identityScope.value = 'tenant:member:actor:session-two';
  await nextTick();
  expect(wrapper.text()).not.toContain('fixture_notice');
});
