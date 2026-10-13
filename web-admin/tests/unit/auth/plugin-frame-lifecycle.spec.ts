import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick, reactive } from 'vue';
import { captureAuthSession, commitAuthSession } from '~/utils/auth-session';
const bridge=vi.hoisted(()=>({prepareFrame:vi.fn(),recoverFrameAuth:vi.fn(),register:vi.fn(),unregister:vi.fn(),navigateFrame:vi.fn()}));
vi.mock('~/composables/usePluginBridge',()=>({usePluginBridge:()=>bridge}));
import PluginWebView from '~/components/PluginWebView.vue';
import PluginPage from '~/pages/_p/[pluginId]/admin/[...rest].vue';
let wrapper: ReturnType<typeof mount> | undefined;
beforeEach(()=>{
  vi.clearAllMocks();vi.useFakeTimers();vi.stubGlobal('useRuntimeConfig',()=>({public:{}}));
  vi.stubGlobal('useRoute',()=>({query:{}}));vi.stubGlobal('useI18n',()=>({t:(key:string)=>key}));vi.stubGlobal('definePageMeta',vi.fn());
  (process as any).client=true;localStorage.clear();
  commitAuthSession({access_token:`h.${btoa(JSON.stringify({tid:'tenant',exp:Math.floor(Date.now()/1000)+3600}))}.s`,refresh_token:'refresh'});
  // jsdom schedules storage notifications as zero-delay timers for these setup writes.
  vi.advanceTimersByTime(0);
  bridge.recoverFrameAuth.mockReset().mockResolvedValue(null);
});
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.useRealTimers();vi.unstubAllGlobals();});
const create=()=>wrapper=mount(PluginWebView,{props:{src:'/_p/com.powerx.plugins.ai-craft/admin/current?__px_iframe=1',pluginId:'com.powerx.plugins.ai-craft',navigatePath:'/current',constrainToViewport:true}});
const rejectDocument=(w:ReturnType<typeof mount>)=>{
  const body=document.createElement('body');body.setAttribute('data-powerx-plugin-frame-error','PLUGIN_FRAME_AUTH_REQUIRED');
  Object.defineProperty(w.find('iframe').element,'contentWindow',{configurable:true,value:{document:{body}}});
};
describe('plugin frame lifecycle',()=>{
  it('automatically restores a rejected document once without asking the user to log in',async()=>{
    bridge.recoverFrameAuth.mockResolvedValue(captureAuthSession());
    const w=create();rejectDocument(w);
    await w.find('iframe').trigger('load');await nextTick();
    expect(bridge.recoverFrameAuth).toHaveBeenCalledTimes(1);
    expect(w.find('iframe').attributes('src')).toContain('__px_retry=1');
    expect(bridge.register).toHaveBeenCalledTimes(2);
    expect(w.find('button').exists()).toBe(false);
  });
  it('stops after the one automatic authentication recovery attempt',async()=>{
    bridge.recoverFrameAuth.mockResolvedValue(captureAuthSession());
    const w=create();rejectDocument(w);
    await w.find('iframe').trigger('load');await nextTick();
    await w.find('iframe').trigger('load');await nextTick();
    expect(bridge.recoverFrameAuth).toHaveBeenCalledTimes(1);
    expect(w.find('iframe').attributes('src')).toContain('__px_retry=1');
    expect(w.text()).toContain('pluginFrame.errors.PLUGIN_FRAME_AUTH_REQUIRED');
    expect(w.find('.animate-pulse').exists()).toBe(false);
  });
  it('does not reload an unmounted frame when recovery completes late',async()=>{
    let resolve!:(stamp:ReturnType<typeof captureAuthSession>)=>void;
    bridge.recoverFrameAuth.mockReturnValue(new Promise(yes=>{resolve=yes}));
    const stamp=captureAuthSession(),w=create();rejectDocument(w);
    await w.find('iframe').trigger('load');
    w.unmount();wrapper=undefined;
    resolve(stamp);await Promise.resolve();await nextTick();
    expect(bridge.register).toHaveBeenCalledTimes(1);
  });
  it('does not reload a replacement document when an old recovery completes',async()=>{
    let resolve!:(stamp:ReturnType<typeof captureAuthSession>)=>void;
    bridge.recoverFrameAuth.mockReturnValue(new Promise(yes=>{resolve=yes}));
    const stamp=captureAuthSession(),w=create();rejectDocument(w);
    await w.find('iframe').trigger('load');
    await w.setProps({src:'/_p/com.powerx.plugins.ai-craft/admin/new?__px_iframe=1',navigatePath:'/new'});
    resolve(stamp);await Promise.resolve();await nextTick();
    expect(w.find('iframe').attributes('src')).toContain('/admin/new');
    expect(w.find('iframe').attributes('src')).not.toContain('__px_retry');
    expect(bridge.register).toHaveBeenCalledTimes(2);
  });
  it('keeps transient host renewal failures retryable without reloading the document',async()=>{
    bridge.recoverFrameAuth.mockRejectedValue({status:503});
    const w=create();rejectDocument(w);
    await w.find('iframe').trigger('load');await nextTick();
    expect(w.text()).toContain('pluginFrame.errors.PLUGIN_HOST_AUTH_UNAVAILABLE');
    expect(w.find('iframe').attributes('src')).not.toContain('__px_retry');
    expect(bridge.register).toHaveBeenCalledTimes(1);
  });
  it('prepares host credentials before the first document and a manual retry',async()=>{
    const w=create();
    expect(bridge.prepareFrame).toHaveBeenCalledTimes(1);
    expect(bridge.prepareFrame.mock.invocationCallOrder[0]).toBeLessThan(bridge.register.mock.invocationCallOrder[0]);
    bridge.register.mock.calls[0][1].onStatus({state:'error',code:'PLUGIN_FRAME_AUTH_REQUIRED'});
    await nextTick();
    await w.find('button').trigger('click');
    expect(bridge.prepareFrame).toHaveBeenCalledTimes(2);
    expect(w.find('iframe').attributes('src')).toContain('__px_retry=1');
  });
  it('keeps the frame and src stable while routing through the bridge',async()=>{
    const w=create(),frame=w.find('iframe').element,src=w.find('iframe').attributes('src');
    await w.setProps({navigatePath:'/other?q=1#part'});
    expect(w.find('iframe').element).toBe(frame);expect(w.find('iframe').attributes('src')).toBe(src);
    expect(bridge.navigateFrame).toHaveBeenLastCalledWith(frame,'/other?q=1#part');expect(bridge.register).toHaveBeenCalledTimes(1);
  });
  it('does not treat load as readiness, and displays a checked bridge failure',async()=>{
    const w=create();await w.find('iframe').trigger('load');
    expect(w.find('.animate-pulse').exists()).toBe(true);
    bridge.register.mock.calls[0][1].onStatus({state:'error',code:'PLUGIN_BRIDGE_TIMEOUT'});await nextTick();
    expect(w.find('.animate-pulse').exists()).toBe(false);expect(w.text()).toContain('pluginFrame.errors.PLUGIN_BRIDGE_TIMEOUT');
    expect(w.find('iframe').attributes('src')).not.toContain('__px_retry');
  });
  it('shows a document failure when host recovery is unavailable and stops bridge waiting',async()=>{
    const w=create(),frame=w.find('iframe').element as HTMLIFrameElement;
    const body=document.createElement('body');
    body.setAttribute('data-powerx-plugin-frame-error','PLUGIN_FRAME_AUTH_REQUIRED');
    Object.defineProperty(frame,'contentWindow',{configurable:true,value:{document:{body}}});
    await w.find('iframe').trigger('load');
    expect(w.text()).toContain('pluginFrame.errors.PLUGIN_FRAME_AUTH_REQUIRED');
    expect(w.find('.animate-pulse').exists()).toBe(false);
    expect(bridge.unregister).toHaveBeenCalledWith(frame);
    expect(w.find('iframe').attributes('src')).not.toContain('__px_retry');
    expect(vi.getTimerCount()).toBe(0);
  });
  it('bounds explicit upstream-not-ready document retries and stops timers on unmount',async()=>{
    const w=create(),frame=w.find('iframe').element as HTMLIFrameElement;
    Object.defineProperty(frame,'contentWindow',{configurable:true,value:{document:{body:{innerText:'PLUGIN_ADMIN_UPSTREAM_NOT_READY'}}}});
    for(let i=0;i<12;i++){await w.find('iframe').trigger('load');vi.advanceTimersByTime(1000);await nextTick();}
    await w.find('iframe').trigger('load');expect(w.text()).toContain('pluginFrame.errors.PLUGIN_ADMIN_UPSTREAM_NOT_READY');
    expect(w.find('.animate-pulse').exists()).toBe(false);const src=w.find('iframe').attributes('src');
    vi.advanceTimersByTime(20000);await nextTick();expect(w.find('iframe').attributes('src')).toBe(src);
    expect(bridge.unregister).toHaveBeenCalled();w.unmount();wrapper=undefined;expect(vi.getTimerCount()).toBe(0);
  });
  it('captures the deep link once instead of navigating by document src',async()=>{
    const route=reactive({params:{pluginId:'com.powerx.plugins.ai-craft'},fullPath:'/_p/com.powerx.plugins.ai-craft/admin/current?q=1#part'});
    vi.stubGlobal('useRoute',()=>route);vi.stubGlobal('computed', (await import('vue')).computed);
    wrapper=mount(PluginPage,{global:{stubs:{PluginWebView:{props:['src','navigatePath','pluginId'],template:'<div :data-src="src" :data-path="navigatePath" />'}}}});
    const src=wrapper.find('div').attributes('data-src');route.fullPath='/_p/com.powerx.plugins.ai-craft/admin/other?q=2';await nextTick();
    expect(wrapper.find('div').attributes('data-src')).toBe(src);expect(wrapper.find('div').attributes('data-path')).toBe('/other?q=2');
  });
});
