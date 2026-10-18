import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, nextTick, reactive, type EffectScope } from 'vue';
import { useNotifications } from '~/composables/useNotifications';
import { AUTH_SESSION_EVENT, AUTH_SESSION_KEY } from '~/utils/auth-session';

const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), markRead: vi.fn(), delete: vi.fn(), clearAll: vi.fn() }));
let user: { user: { id: number }; currentTenantUuid: string; currentMemberUuid: string };
vi.mock('~/stores/user', () => ({ useUserStore: () => user }));
vi.mock('~/composables/api/services/notificationService', () => ({ useNotificationService: () => api }));
const tenant = '6b5d0240-9920-46da-b707-88200e0f51ea';
const a = '11111111-1111-4111-8111-111111111111';
const b = '22222222-2222-4222-8222-222222222222';
let scope: EffectScope;
const deferred = () => { let resolve!: (value: any) => void; const promise = new Promise<any>(r => { resolve=r; }); return {promise,resolve}; };
beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  vi.stubGlobal("navigateTo",vi.fn().mockResolvedValue(undefined));
  user=reactive({ user:{id:1}, currentTenantUuid:tenant, currentMemberUuid:a });
  scope=effectScope();
  api.list.mockResolvedValue({data:{items:[],pagination:{total:0}}});
});
afterEach(() => { scope.stop(); vi.unstubAllGlobals(); localStorage.clear(); });

const commitSession = (sessionId: string, revision = 1) => {
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify({
    version: 1, status: 'active', sessionId, revision, accessToken: 'fixture',
    refreshToken: 'fixture', tokenType: 'Bearer', expiresAt: Date.now() + 60000,
    scope: 'access', tenantUuid: tenant,
  }));
};

describe('personal notification isolation', () => {
  it.each(['local', 'storage'])('rebinds the same account after a new %s login session', async event => {
    vi.stubGlobal('process', { ...process, client: true });
    commitSession('session-one');
    api.markRead.mockResolvedValue({});
    const bell = scope.run(useNotifications)!, detail = scope.run(useNotifications)!;
    const old = deferred();
    api.list.mockReturnValueOnce(old.promise);
    const request = bell.fetchNotifications();
    const id = '00000000-0000-4000-8000-000000000010';
    await bell.addNotification({ id, userId: a, isRead: false });
    await detail.markAsRead(id);
    const firstScope = bell.identityScope.value;
    commitSession('session-two');
    window.dispatchEvent(event === 'local' ? new Event(AUTH_SESSION_EVENT) : new StorageEvent('storage', { key: AUTH_SESSION_KEY }));
    expect(bell.identityScope.value).not.toBe(firstScope);
    expect(bell.notifications.value).toHaveLength(0);
    await bell.addNotification({ id, userId: a, isRead: false });
    expect(bell.getStats().unread).toBe(1);
    await detail.markAsRead(id);
    expect(bell.getStats().unread).toBe(0);
    old.resolve({ data: { items: [{ id: 'old', userId: a }], pagination: { total: 1 } } });
    await request;
    expect(bell.notifications.value.map(n => n.id)).toEqual([id]);
  });

  it('keeps receipts during token refresh and releases session listeners on disposal', async () => {
    vi.stubGlobal('process', { ...process, client: true });
    commitSession('session-one');
    const n = scope.run(useNotifications)!;
    const id = '00000000-0000-4000-8000-000000000011';
    await n.addNotification({ id, userId: a });
    await n.markAsRead(id);
    commitSession('session-one', 2);
    window.dispatchEvent(new Event(AUTH_SESSION_EVENT));
    expect(n.getStats().unread).toBe(0);
    expect(n.notifications.value).toHaveLength(1);
    const before = n.identityScope.value;
    scope.stop();
    commitSession('session-two');
    window.dispatchEvent(new Event(AUTH_SESSION_EVENT));
    expect(n.identityScope.value).toBe(before);
  });

  it('coalesces receipts into one filtered refresh and ignores an older list response', async () => {
    const stale = deferred();
    api.list.mockReturnValueOnce(stale.promise);
    const n = scope.run(useNotifications)!;
    n.setFilter({ isRead: false });
    const id = '00000000-0000-4000-8000-000000000012';
    const event = { eventType: 'notification_read', payload_version: 1, contract: 'notification_read_receipt.v1', tenant_uuid: tenant, userId: a, notification_uuids: [id] };
    n.applyReadReceipt(event);
    n.applyReadReceipt(event);
    await nextTick();
    await Promise.resolve();
    expect(api.list).toHaveBeenCalledTimes(2);
    stale.resolve({ data: { items: [{ id, userId: a, isRead: false }], pagination: { total: 1 } } });
    await stale.promise;
    await nextTick();
    expect(n.notifications.value).toHaveLength(0);
    expect(n.totalCount.value).toBe(0);
    n.applyReadReceipt(event);
    await nextTick();
    expect(api.list).toHaveBeenCalledTimes(2);
  });

  it('reconciles off-page read receipts with the filtered server total', async () => {
    const id = '00000000-0000-4000-8000-000000000013';
    api.list.mockResolvedValueOnce({ data: { items: [], pagination: { total: 3 } } });
    const n = scope.run(useNotifications)!;
    n.setFilter({ isRead: false });
    await nextTick();
    expect(n.totalCount.value).toBe(3);
    api.list.mockResolvedValueOnce({ data: { items: [], pagination: { total: 2 } } });
    n.applyReadReceipt({ eventType: 'notification_read', payload_version: 1, contract: 'notification_read_receipt.v1', tenant_uuid: tenant, userId: a, notification_uuids: [id] });
    await nextTick();
    await Promise.resolve();
    expect(n.totalCount.value).toBe(2);
  });

  it('removes confirmed stale unread rows from the response total', async () => {
    const id = '00000000-0000-4000-8000-000000000014';
    const n = scope.run(useNotifications)!;
    n.applyReadReceipt({ eventType: 'notification_read', payload_version: 1, contract: 'notification_read_receipt.v1', tenant_uuid: tenant, userId: a, notification_uuids: [id] });
    api.list.mockResolvedValueOnce({ data: { items: [{ id, userId: a, isRead: false }], pagination: { total: 1 } } });
    n.setFilter({ isRead: false });
    await nextTick();
    expect(n.notifications.value).toHaveLength(0);
    expect(n.totalCount.value).toBe(0);
  });

  it('shares confirmed read mutations between bell/list/detail and protects later notices', async () => {
    api.markRead.mockResolvedValue({});
    const bell=scope.run(useNotifications)!, detail=scope.run(useNotifications)!;
    const id='00000000-0000-4000-8000-000000000001', later='00000000-0000-4000-8000-000000000002';
    await bell.addNotification({id,userId:a,isRead:false});await bell.addNotification({id:later,userId:a,isRead:false});
    await detail.markAsRead(id);
    expect(bell.getStats().unread).toBe(1);
    const event={eventType:'notification_read',payload_version:1,contract:'notification_read_receipt.v1',tenant_uuid:tenant,userId:a,notification_uuids:[later]};
    bell.applyReadReceipt({...event,userId:b});expect(bell.getStats().unread).toBe(1);
    bell.applyReadReceipt(event);expect(bell.getStats().unread).toBe(0);expect(bell.notifications.value).toHaveLength(2);
    api.list.mockResolvedValueOnce({data:{items:[{id:later,userId:a,isRead:false}],pagination:{total:1}}});
    await bell.fetchNotifications();expect(bell.getStats().unread).toBe(0);
    user.currentMemberUuid=b;
    await bell.addNotification({id:later,userId:b,isRead:false});bell.applyReadReceipt(event);
    expect(bell.getStats().unread).toBe(1);
  });
  it('accepts a read receipt arriving before its create event without resurrecting unread state', async () => {
    const bell=scope.run(useNotifications)!;const id='00000000-0000-4000-8000-000000000003';
    bell.applyReadReceipt({eventType:'notification_read',payload_version:1,contract:'notification_read_receipt.v1',tenant_uuid:tenant,userId:a,notification_uuids:[id]});
    await bell.addNotification({id,userId:a,isRead:false});expect(bell.getStats().unread).toBe(0);
  });
  it('opens a guarded business entry from the bell and does not call any business read API', async () => {
    api.markRead.mockResolvedValue({});
    const n=scope.run(useNotifications)!;
    const path='/_p/plugin.test/admin/order?target=opaque';
    const notice={id:'business',userId:a,actions:[{id:'view_business',label:'fixture_action',type:'primary' as const,action:'navigate',params:{path}}]};
    await n.addNotification(notice);
    await n.openNotification('business');
    expect(navigateTo).toHaveBeenCalledWith(path);
    expect(api.markRead).toHaveBeenCalledWith('business');
    // A detail panel has its own composable, so its displayed record is explicit.
    const detail=scope.run(useNotifications)!;
    await detail.executeAction('business','view_business',n.notifications.value[0]);
    expect(navigateTo).toHaveBeenCalledTimes(2);
    const oldNotice=n.notifications.value[0];
    user.currentMemberUuid=b;
    await detail.executeAction('business','view_business',oldNotice);
    expect(navigateTo).toHaveBeenCalledTimes(2);
  });
  it('rejects a bad destination without marking the reminder as read', async () => {
    const n=scope.run(useNotifications)!;
    await n.addNotification({id:'unsafe',userId:a,actions:[{id:'view_business',label:'fixture_action',type:'primary',action:'navigate',params:{path:'//evil.test'}}]});
    await expect(n.openNotification('unsafe')).rejects.toThrow('NOTIFICATION_NAVIGATION_INVALID');
    expect(api.markRead).not.toHaveBeenCalled();
    expect(navigateTo).not.toHaveBeenCalled();
  });
  it('only accepts the current member and explicit broadcast; reconnect duplicates do not inflate counts', async () => {
    const n=scope.run(useNotifications)!;
    await n.addNotification({id:'b',userId:b});
    expect(n.notifications.value).toHaveLength(0);
    await n.addNotification({id:'a',userId:a});
    await n.addNotification({id:'a',userId:a});
    await n.addNotification({id:'broadcast'});
    expect(n.notifications.value.map(v=>v.id)).toEqual(['broadcast','a']);
    expect(n.totalCount.value).toBe(2);
    user.currentMemberUuid=b;
    expect(n.notifications.value).toHaveLength(0);
    expect(n.totalCount.value).toBe(0);
    await n.addNotification({id:'b',userId:b});
    expect(n.notifications.value[0]?.id).toBe('b');
  });
  it('ignores an old identity list response, including old broadcasts', async () => {
    const pending=deferred();api.list.mockReturnValueOnce(pending.promise);
    const n=scope.run(useNotifications)!;
    const read=n.fetchNotifications();
    user.currentMemberUuid=b;
    pending.resolve({data:{items:[{id:'old',title:'old broadcast'}],pagination:{total:1}}});
    await read;
    expect(n.notifications.value).toHaveLength(0);
    expect(n.totalCount.value).toBe(0);
    expect(n.loading.value).toBe(false);
  });
  it('does not let an old mark-read completion overwrite a new identity notification', async () => {
    const pending=deferred();api.markRead.mockReturnValueOnce(pending.promise);
    const n=scope.run(useNotifications)!;
    await n.addNotification({id:'same',userId:a,isRead:false});
    const update=n.markAsRead('same');
    user.user.id=2;user.currentMemberUuid=b;
    await n.addNotification({id:'same',userId:b,isRead:false});
    pending.resolve({});await update;
    expect(n.notifications.value[0]?.isRead).toBe(false);
    expect(n.notifications.value[0]?.userId).toBe(b);
  });
  it('drops caches on tenant change and filters foreign history records', async () => {
    const n=scope.run(useNotifications)!;
    await n.addNotification({id:'old',userId:a});
    user.currentTenantUuid='33333333-3333-4333-8333-333333333333';
    expect(n.notifications.value).toHaveLength(0);
    api.list.mockResolvedValueOnce({data:{items:[{id:'foreign',userId:b},{id:'mine',userId:a}],pagination:{total:2}}});
    await n.fetchNotifications();
    expect(n.notifications.value.map(v=>v.id)).toEqual(['mine']);
  });
});
