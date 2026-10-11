import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick, reactive } from 'vue';
const bridge=vi.hoisted(()=>({register:vi.fn(),unregister:vi.fn(),navigateFrame:vi.fn()}));
vi.mock('~/composables/usePluginBridge',()=>({usePluginBridge:()=>bridge}));
import PluginWebView from '~/components/PluginWebView.vue';
import PluginPage from '~/pages/_p/[pluginId]/admin/[...rest].vue';
let wrapper: ReturnType<typeof mount> | undefined;
beforeEach(()=>{
  vi.clearAllMocks();vi.useFakeTimers();vi.stubGlobal('useRuntimeConfig',()=>({public:{}}));
  vi.stubGlobal('useRoute',()=>({query:{}}));vi.stubGlobal('useI18n',()=>({t:(key:string)=>key}));vi.stubGlobal('definePageMeta',vi.fn());
});
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.useRealTimers();vi.unstubAllGlobals();});
const create=()=>wrapper=mount(PluginWebView,{props:{src:'/_p/com.powerx.plugins.ai-craft/admin/current?__px_iframe=1',pluginId:'com.powerx.plugins.ai-craft',navigatePath:'/current',constrainToViewport:true}});
describe('plugin frame lifecycle',()=>{
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
