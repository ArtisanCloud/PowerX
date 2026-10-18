import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { effectScope, nextTick, reactive, ref, type EffectScope } from 'vue';
import { AUTH_SESSION_EVENT, AUTH_SESSION_KEY } from '~/utils/auth-session';

const auth = vi.hoisted(() => ({ token: null as any, getToken: vi.fn(), refreshAuth: vi.fn() }));
let user: any;
vi.mock('~/composables/useAuth', () => ({ useAuth: () => auth }));
vi.mock('~/stores/user', () => ({ useUserStore: () => user }));
const tenant = '6b5d0240-9920-46da-b707-88200e0f51ea';
class Socket {
  static OPEN = 1;
  static instances: Socket[]=[];
  readyState=1;
  onopen?: () => void;
  onclose?: (event: any) => void;
  onerror?: () => void;
  onmessage?: (event: any) => void;
  send=vi.fn();
  close=vi.fn();
  constructor(public url: string) { Socket.instances.push(this); }
  event() { this.onmessage?.({data:JSON.stringify({type:'event',topic:'_topic.system.notification',payload:{id:'notice'}})}); }
}
let scope: EffectScope;
const scopes: EffectScope[] = [];
beforeEach(() => {
  vi.resetModules();Socket.instances=[];scope=effectScope();
  auth.token=ref('first-token');
  auth.getToken.mockImplementation(() => auth.token.value);
  user=reactive({currentTenantUuid:tenant});
  vi.stubGlobal('process',{...process,client:true});
  vi.stubGlobal('WebSocket',Socket);
  vi.stubGlobal('useRuntimeConfig',()=>({public:{wsPath:'/api/ws'}}));
  vi.stubGlobal('useToast',()=>({add:vi.fn()}));
  vi.stubGlobal('useI18n',()=>({t:(key:string)=>key}));
  localStorage.setItem(AUTH_SESSION_KEY,JSON.stringify({version:1,status:'active',sessionId:'session-one',revision:1,accessToken:'first-token',refreshToken:'refresh',tokenType:'Bearer',expiresAt:Date.now()+60000,scope:'access',tenantUuid:tenant}));
});
afterEach(()=>{scope.stop();scopes.splice(0).forEach(s => s.stop());vi.unstubAllGlobals();localStorage.clear()});

it('ignores old socket frames after reconnect and closes the last subscription',async()=>{
  const {useWSBus}=await import('~/composables/useWSBus');
  const bus=scope.run(useWSBus)!;
  const received=vi.fn();const unsubscribe=bus.subscribe('_topic.system.notification',received);
  const first=Socket.instances[0]!;first.onopen?.();first.event();expect(received).toHaveBeenCalledTimes(1);
  auth.token.value='second-token';await nextTick();
  const second=Socket.instances.at(-1)!;expect(second).not.toBe(first);second.onopen?.();
  first.event();expect(received).toHaveBeenCalledTimes(1);
  second.event();expect(received).toHaveBeenCalledTimes(2);
  const session=JSON.parse(localStorage.getItem(AUTH_SESSION_KEY)!);session.sessionId='session-two';localStorage.setItem(AUTH_SESSION_KEY,JSON.stringify(session));
  second.event();expect(received).toHaveBeenCalledTimes(2);
  unsubscribe();expect(second.close).toHaveBeenCalled();
});

it('reinstalls identity watches after the first Header scope is disposed', async () => {
  const { useWSBus } = await import('~/composables/useWSBus');
  const firstBus = scope.run(useWSBus)!;
  firstBus.subscribe('_topic.system.notification', vi.fn())();
  scope.stop();
  const replacement = effectScope(); scopes.push(replacement);
  const bus = replacement.run(useWSBus)!;
  const unsubscribe = bus.subscribe('_topic.system.notification', vi.fn());
  const before = Socket.instances.at(-1)!; before.onopen?.();
  auth.token.value = 'new-login-token';
  await nextTick();
  expect(before.close).toHaveBeenCalledOnce();
  expect(Socket.instances.at(-1)).not.toBe(before);
  unsubscribe();
});

it.each(['local', 'storage'])('rebinds a new %s session even when its access token is unchanged', async event => {
  const { useWSBus } = await import('~/composables/useWSBus');
  const bus = scope.run(useWSBus)!;
  const received = vi.fn();
  const unsubscribe = bus.subscribe('_topic.system.notification', received);
  const old = Socket.instances.at(-1)!; old.onopen?.();
  const session = JSON.parse(localStorage.getItem(AUTH_SESSION_KEY)!);
  session.sessionId = 'session-two';
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify(session));
  window.dispatchEvent(event === 'local' ? new Event(AUTH_SESSION_EVENT) : new StorageEvent('storage', { key: AUTH_SESSION_KEY }));
  await nextTick();
  expect(old.close).toHaveBeenCalledOnce();
  const current = Socket.instances.at(-1)!;
  expect(current).not.toBe(old); current.onopen?.();
  old.event(); current.event();
  expect(received).toHaveBeenCalledOnce();
  unsubscribe();
});

it('keeps the shared socket alive for remaining consumers and releases all listeners on disposal', async () => {
  const { useWSBus } = await import('~/composables/useWSBus');
  const first = scope.run(useWSBus)!;
  first.subscribe('_topic.system.notification', vi.fn());
  const secondScope = effectScope(); scopes.push(secondScope);
  const second = secondScope.run(useWSBus)!;
  const received = vi.fn(); second.subscribe('_topic.system.notification', received);
  const socket = Socket.instances.at(-1)!; socket.onopen?.();
  expect(Socket.instances).toHaveLength(1);
  scope.stop();
  expect(socket.close).not.toHaveBeenCalled();
  socket.event(); expect(received).toHaveBeenCalledOnce();
  secondScope.stop();
  expect(socket.close).toHaveBeenCalledOnce();
  const count = Socket.instances.length;
  window.dispatchEvent(new Event('online'));
  window.dispatchEvent(new Event(AUTH_SESSION_EVENT));
  auth.token.value = 'after-disposal'; await nextTick();
  expect(Socket.instances).toHaveLength(count);
});

it('rebinds tenant changes and rejects late open/message callbacks from the old connection', async () => {
  const { useWSBus } = await import('~/composables/useWSBus');
  const bus = scope.run(useWSBus)!;
  const received = vi.fn(); bus.subscribe('_topic.system.notification', received);
  const old = Socket.instances.at(-1)!;
  const session = JSON.parse(localStorage.getItem(AUTH_SESSION_KEY)!);
  session.tenantUuid = '33333333-3333-4333-8333-333333333333';
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify(session));
  window.dispatchEvent(new Event(AUTH_SESSION_EVENT)); await nextTick();
  const current = Socket.instances.at(-1)!;
  expect(current).not.toBe(old);
  old.onopen?.(); old.event(); expect(received).not.toHaveBeenCalled();
  current.onopen?.(); current.event(); expect(received).toHaveBeenCalledOnce();
  expect(current.url).toContain(session.tenantUuid);
});
