import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PluginBridgeHost } from '~/utils/plugin-bridge-host';
import { clearAuthSession, commitAuthSession } from '~/utils/auth-session';
let host: PluginBridgeHost, frame: HTMLIFrameElement;
let post: ReturnType<typeof vi.fn>, status: ReturnType<typeof vi.fn>;
const pluginId='com.powerx.plugins.ai-craft', instanceId='frame-1';
const send = (type: string, fields: Record<string,unknown>={}, origin='http://localhost:8110') => window.dispatchEvent(new MessageEvent('message',{
  source:frame.contentWindow, origin, data:{source:'plugin',protocolVersion:2,pluginId,instanceId,type,...fields}
}));
const packets = (type: string) => post.mock.calls.map(args=>args[0]).filter(item=>item.type===type);
beforeEach(()=>{
  (process as any).client=true;localStorage.clear();vi.useFakeTimers();
  frame=document.createElement('iframe'); frame.src='http://localhost:8110/_p/com.powerx.plugins.ai-craft/admin/current';document.body.append(frame);
  post=vi.fn();status=vi.fn();vi.spyOn(frame.contentWindow!,'postMessage').mockImplementation(post);
  const access=`h.${btoa(JSON.stringify({tid:'tenant',exp:Math.floor(Date.now()/1000)+3600}))}.s`;
  commitAuthSession({access_token:access,refresh_token:'refresh',expires_in:3600});
  host=new PluginBridgeHost(()=>({locale:'zh',theme:'dark'}),1000);host.register(frame,pluginId,instanceId,status);
});
afterEach(()=>{host.dispose();frame.remove();vi.useRealTimers();vi.restoreAllMocks();});
describe('host bridge contract v2',()=>{
  it('uses the registered exact target origin and exposes credentials only after a verified handshake',()=>{
    expect(packets('auth-token')).toHaveLength(0);
    send('bridge:ready');expect(packets('auth-token')).toHaveLength(1);
    expect(packets('auth-token')[0].refreshToken).toBeUndefined();
    expect(post.mock.calls.every(args=>args[1]==='http://localhost:8110')).toBe(true);
  });
  it('rejects wrong origins, plugin identities, instances and old protocol handshakes',()=>{
    send('bridge:ready',{},'https://other.example');send('bridge:ready',{pluginId:'other'});
    send('bridge:ready',{instanceId:'old'});send('bridge:ready',{protocolVersion:1});
    expect(packets('auth-token')).toHaveLength(0);
    vi.advanceTimersByTime(1000);expect(status).toHaveBeenLastCalledWith({state:'error',code:'PLUGIN_BRIDGE_TIMEOUT'});
  });
  it('allows explicit resynchronization of the same token',()=>{
    send('bridge:ready');send('auth-token:request');expect(packets('auth-token')).toHaveLength(2);
    expect(packets('auth-token')[1].sequence).toBeGreaterThan(packets('auth-token')[0].sequence);
  });
  it('publishes signed-out state without exposing an old credential',()=>{
    send('bridge:ready');clearAuthSession();
    const cleared=packets('auth-cleared').at(-1);expect(cleared).toBeDefined();expect(cleared.accessToken).toBeUndefined();
  });
  it('queues the latest path before readiness without changing iframe src',()=>{
    const src=frame.src;host.navigate(frame,'/one');host.navigate(frame,'/two');send('bridge:ready');
    expect(packets('navigate')).toHaveLength(1);expect(packets('navigate')[0].path).toBe('/two');expect(frame.src).toBe(src);
  });
  it('ignores late navigation acknowledgements and requires current request success',()=>{
    send('bridge:ready');const auth=packets('auth-token').at(-1);send('auth:applied',{sequence:auth.sequence,sessionId:auth.sessionId,revision:auth.revision});host.navigate(frame,'/one');const one=packets('navigate').at(-1);
    host.navigate(frame,'/two');const two=packets('navigate').at(-1);
    send('navigation:result',{requestId:one.requestId,ok:true});expect(status).toHaveBeenLastCalledWith({state:'loading'});
    send('navigation:result',{requestId:two.requestId,ok:true});expect(status).toHaveBeenLastCalledWith({state:'ready'});
    host.navigate(frame,'/two');expect(packets('navigate')).toHaveLength(2);
  });
  it('sends a new request when returning to a completed path during another navigation',()=>{
    send('bridge:ready',{navigationReady:true});
    const auth=packets('auth-token').at(-1);
    send('auth:applied',{sequence:auth.sequence,sessionId:auth.sessionId,revision:auth.revision});
    host.navigate(frame,'/tracks');
    send('navigation:result',{requestId:packets('navigate').at(-1).requestId,ok:true});
    host.navigate(frame,'/bulk-orders');const bulk=packets('navigate').at(-1);
    host.navigate(frame,'/tracks');const returned=packets('navigate').at(-1);
    expect(returned.path).toBe('/tracks');expect(returned.requestId).not.toBe(bulk.requestId);
    send('navigation:result',{requestId:bulk.requestId,ok:false});
    expect(status).toHaveBeenLastCalledWith({state:'loading'});
    send('navigation:result',{requestId:returned.requestId,ok:true});
    expect(status).toHaveBeenLastCalledWith({state:'ready'});
  });
  it('joins a repeated pending navigation instead of cancelling the Router operation',()=>{
    send('bridge:ready',{navigationReady:true});host.navigate(frame,'/bulk-orders');
    const request=packets('navigate').at(-1);host.navigate(frame,'/bulk-orders');
    expect(packets('navigate')).toHaveLength(1);
    expect(packets('navigate').at(-1).requestId).toBe(request.requestId);
  });
  it('reconfirms the route after a failure instead of retaining a stale error',()=>{
    send('bridge:ready',{navigationReady:true});const auth=packets('auth-token').at(-1);
    send('auth:applied',{sequence:auth.sequence,sessionId:auth.sessionId,revision:auth.revision});
    host.navigate(frame,'/tracks');
    send('navigation:result',{requestId:packets('navigate').at(-1).requestId,ok:true});
    host.navigate(frame,'/bulk-orders');
    send('navigation:result',{requestId:packets('navigate').at(-1).requestId,ok:false});
    host.navigate(frame,'/tracks');
    expect(packets('navigate').at(-1).path).toBe('/tracks');
    send('navigation:result',{requestId:packets('navigate').at(-1).requestId,ok:true});
    expect(status).toHaveBeenLastCalledWith({state:'ready'});
  });
  it('does not classify a Router failure as a missing navigation adapter',()=>{
    send('bridge:ready',{navigationReady:true});host.navigate(frame,'/bulk-orders');
    send('navigation:result',{requestId:packets('navigate').at(-1).requestId,ok:false});
    expect(status).toHaveBeenLastCalledWith({state:'error',code:'PLUGIN_NAVIGATION_FAILED'});
  });
  it('requires fresh route confirmation after an iframe document reload',()=>{
    send('bridge:ready',{navigationReady:true});const auth=packets('auth-token').at(-1);
    send('auth:applied',{sequence:auth.sequence,sessionId:auth.sessionId,revision:auth.revision});
    host.navigate(frame,'/tracks');
    send('navigation:result',{requestId:packets('navigate').at(-1).requestId,ok:true});
    frame.dispatchEvent(new Event('load'));host.navigate(frame,'/tracks');
    send('bridge:ready',{navigationReady:true});const nav=packets('navigate').at(-1);
    expect(packets('navigate')).toHaveLength(2);
    expect(status).toHaveBeenLastCalledWith({state:'loading'});
    send('navigation:result',{requestId:nav.requestId,ok:true});
    const latest=packets('auth-token').at(-1);
    send('auth:applied',{sequence:latest.sequence,sessionId:latest.sessionId,revision:latest.revision});
    expect(status).toHaveBeenLastCalledWith({state:'ready'});
  });
  it('shows an unsupported navigator result without reloading the document',()=>{
    send('bridge:ready');const src=frame.src;host.navigate(frame,'/other');
    send('navigation:result',{requestId:packets('navigate').at(-1).requestId,ok:false});
    expect(status).toHaveBeenLastCalledWith({state:'error',code:'PLUGIN_NAVIGATION_UNSUPPORTED'});expect(frame.src).toBe(src);
  });
  it('unregister cancels timers and pending callbacks',()=>{
    host.unregister(frame);const calls=status.mock.calls.length;vi.advanceTimersByTime(5000);send('bridge:ready');
    expect(status).toHaveBeenCalledTimes(calls);expect(packets('auth-token')).toHaveLength(0);
  });
  it('waits for the current auth adapter as well as route success',()=>{
    host.navigate(frame,'/current');send('bridge:ready');const nav=packets('navigate').at(-1),auth=packets('auth-token').at(-1);
    send('navigation:result',{requestId:nav.requestId,ok:true});expect(status).toHaveBeenLastCalledWith({state:'loading'});
    send('auth:applied',{sequence:auth.sequence-1,sessionId:auth.sessionId,revision:auth.revision});expect(status).toHaveBeenLastCalledWith({state:'loading'});
    send('auth:applied',{sequence:auth.sequence,sessionId:auth.sessionId,revision:auth.revision});expect(status).toHaveBeenLastCalledWith({state:'ready'});
  });
  it('fails visibly when a plugin does not apply the host session',()=>{
    host.navigate(frame,'/current');send('bridge:ready');send('navigation:result',{requestId:packets('navigate').at(-1).requestId,ok:true});
    vi.advanceTimersByTime(1000);expect(status).toHaveBeenLastCalledWith({state:'error',code:'PLUGIN_AUTH_ADAPTER_TIMEOUT'});
  });
  it('does not repeat initialization when only locale or theme metadata changes',()=>{
    send('bridge:ready');const count=packets('auth-token').length;host.broadcastMetadata();send('bridge:ready');
    expect(packets('auth-token')).toHaveLength(count);
  });
  it('refreshes expired sessions only through the host owner when requested',async()=>{
    host.dispose();const refresh=vi.fn(async()=>{const session=JSON.parse(localStorage.getItem('powerx.session')!);session.expiresAt=Date.now()+3600000;session.revision+=1;localStorage.setItem('powerx.session',JSON.stringify(session));});
    host=new PluginBridgeHost(()=>({locale:'zh',theme:'dark'}),1000,refresh);host.register(frame,pluginId,instanceId,status);send('bridge:ready');
    const expired=JSON.parse(localStorage.getItem('powerx.session')!);expired.expiresAt=Date.now()-1;localStorage.setItem('powerx.session',JSON.stringify(expired));
    send('auth-token:request');await Promise.resolve();await Promise.resolve();expect(refresh).toHaveBeenCalledTimes(1);
    expect(packets('auth-token').at(-1).revision).toBe(expired.revision+1);
  });

});
