import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { JSDOM } from 'jsdom';
import { describe, expect, it, vi } from 'vitest';
const source=readFileSync(resolve('public/powerx-bridge-client.js'),'utf8');
const pluginId='com.powerx.plugins.ai-craft', instanceId='frame-1', origin='http://127.0.0.1:3030';
function fixture(){
  const dom=new JSDOM('<html><body></body></html>',{url:`${origin}/_p/${pluginId}/admin/current?__px_iframe=1`,referrer:`${origin}/home`,runScripts:'outside-only'});
  const parent=new JSDOM('',{url:origin}).window;const post=vi.fn();parent.postMessage=post;
  Object.defineProperty(dom.window,'parent',{value:parent});
  dom.window.__NUXT__={config:{app:{baseURL:`/_p/${pluginId}/admin/`}}};
  const message=(type:string,fields:Record<string,unknown>={},sender:any=parent,senderOrigin=origin)=>dom.window.dispatchEvent(new dom.window.MessageEvent('message',{
    source:sender,origin:senderOrigin,data:{source:'powerx',protocolVersion:2,pluginId,instanceId,type,...fields}
  }));
  dom.window.eval(source);
  return{dom,parent,post,message,close:()=>{dom.window.close();parent.close();}};
}
describe('injected consumer v2',()=>{
  it('does not restore or write host storage and cookies at bootstrap',()=>{
    const f=fixture();try{
      f.dom.window.localStorage.setItem('powerx.session','host-snapshot');f.dom.window.localStorage.setItem('access_token','old');
      f.dom.window.document.cookie='access_token=old-cookie;path=/';f.dom.window.eval(source);
      expect(f.dom.window.localStorage.getItem('powerx.session')).toBe('host-snapshot');
      expect(f.dom.window.localStorage.getItem('access_token')).toBe('old');expect(f.dom.window.document.cookie).toContain('old-cookie');
    }finally{f.close();}
  });
  it('rejects wrong origins, source windows, protocols and instances',()=>{
    const f=fixture();try{
      const cb=vi.fn();f.dom.window.PowerXBridgeClient.onAuthToken(cb);f.message('sync');
      f.message('auth-token',{sequence:1},f.parent,'https://other.example');
      f.message('auth-token',{sequence:1},f.dom.window);f.message('auth-token',{sequence:1,protocolVersion:1});
      f.message('auth-token',{sequence:1,instanceId:'old'});expect(cb).not.toHaveBeenCalled();
    }finally{f.close();}
  });
  it('accepts fresh sequences and prevents old messages from reaching later listeners',async()=>{
    const f=fixture();try{
      const cb=vi.fn(),later=vi.fn();f.dom.window.PowerXBridgeClient.onAuthToken(cb);f.message('sync');
      f.dom.window.addEventListener('message',later);f.message('auth-token',{sequence:2,accessToken:'current'});
      f.message('auth-token',{sequence:1,accessToken:'old'});
      await Promise.resolve(); await Promise.resolve();
      expect(cb).toHaveBeenCalledTimes(1);expect(later).toHaveBeenCalledTimes(1);
    }finally{f.close();}
  });
  it('acknowledges the current deep link without a Router adapter',()=>{
    const f=fixture();try{
      f.message('sync');f.message('navigate',{requestId:'same',path:'/current'});
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'navigation:result',requestId:'same',ok:true});
    }finally{f.close();}
  });
  it('rejects unsupported navigation without changing the document route',()=>{
    const f=fixture();try{
      f.message('sync');f.message('navigate',{requestId:'other',path:'/other'});
      expect(f.dom.window.location.pathname).toContain('/current');
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'navigation:result',ok:false});
    }finally{f.close();}
  });
  it('delegates navigation to the registered Router adapter and acknowledges its actual route',async()=>{
    const f=fixture();try{
      f.message('sync');const adapter=vi.fn((path:string)=>f.dom.window.history.pushState(null,'',path));
      f.dom.window.PowerXBridgeClient.setNavigator(adapter);f.message('navigate',{requestId:'go',path:'/other?q=1#part'});
      await Promise.resolve();await Promise.resolve();await Promise.resolve();
      expect(adapter).toHaveBeenCalledWith(`/_p/${pluginId}/admin/other?q=1#part`,expect.objectContaining({requestId:"go",instanceId}));
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'navigation:result',requestId:'go',ok:true});
    }finally{f.close();}
  });
  it('keeps the plugin mount after Nuxt hydration removes its bootstrap global',async()=>{
    const f=fixture();try{
      f.message('sync');
      const adapter=vi.fn((path:string)=>f.dom.window.history.pushState(null,'',path));
      f.dom.window.PowerXBridgeClient.setNavigator(adapter);
      // Nuxt's revive-payload client plugin removes this global after hydration.
      delete f.dom.window.__NUXT__;
      for (const [index,path] of ['/operations/ai-craft/tracks','/operations/ai-craft/bulk-orders','/operations/ai-craft/tracks'].entries()) {
        const requestId=`hydrated-${index}`;
        f.message('navigate',{requestId,path});
        await new Promise(resolve=>setTimeout(resolve,0));
        expect(f.dom.window.location.pathname).toBe(`/_p/${pluginId}/admin${path}`);
        expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'navigation:result',requestId,ok:true});
      }
      expect(adapter).toHaveBeenCalledTimes(3);
    }finally{f.close();}
  });
  it('uses the verified iframe mount instead of a different runtime config namespace',async()=>{
    const f=fixture();try{
      f.dom.window.__NUXT__.config.app.baseURL='/_p/other/admin/';
      f.message('sync');
      f.dom.window.PowerXBridgeClient.setNavigator((path:string)=>f.dom.window.history.pushState(null,'',path));
      f.message('navigate',{requestId:'namespace',path:'/other'});
      await new Promise(resolve=>setTimeout(resolve,0));
      expect(f.dom.window.location.pathname).toBe(`/_p/${pluginId}/admin/other`);
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({requestId:'namespace',ok:true});
    }finally{f.close();}
  });
  it('accepts a fully scoped root route with query and hash after hydration',async()=>{
    const f=fixture();try{
      delete f.dom.window.__NUXT__;f.message('sync');
      f.dom.window.PowerXBridgeClient.setNavigator((path:string)=>f.dom.window.history.pushState(null,'',path));
      f.message('navigate',{requestId:'root-query',path:`/_p/${pluginId}/admin?q=1#part`});
      await new Promise(resolve=>setTimeout(resolve,0));
      expect(f.dom.window.location.pathname).toBe(`/_p/${pluginId}/admin`);
      expect(f.dom.window.location.search).toBe('?q=1');
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({requestId:'root-query',ok:true});
    }finally{f.close();}
  });
  it.each(['https://other.example/private','//other.example/private','/../../outside'])('rejects an escaping navigation address %s',async(path)=>{
    const f=fixture();try{
      f.message('sync');const adapter=vi.fn();f.dom.window.PowerXBridgeClient.setNavigator(adapter);
      f.message('navigate',{requestId:'escape',path});
      await new Promise(resolve=>setTimeout(resolve,0));
      expect(adapter).not.toHaveBeenCalled();
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({requestId:'escape',ok:false,code:'PLUGIN_NAVIGATION_INVALID'});
    }finally{f.close();}
  });
  it('distinguishes an installed Router failure from a missing adapter',async()=>{
    const f=fixture();try{
      f.message('sync');
      f.dom.window.PowerXBridgeClient.setNavigator(async()=>{throw new Error('PLUGIN_NAVIGATION_FAILED');});
      f.message('navigate',{requestId:'failed',path:'/other'});
      await new Promise(resolve=>setTimeout(resolve,0));
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'navigation:result',requestId:'failed',ok:false,code:'PLUGIN_NAVIGATION_FAILED'});
    }finally{f.close();}
  });
  it('reports a route mismatch when an adapter finishes on another page',async()=>{
    const f=fixture();try{
      f.message('sync');f.dom.window.PowerXBridgeClient.setNavigator(async()=>{});
      f.message('navigate',{requestId:'mismatch',path:'/other'});
      await new Promise(resolve=>setTimeout(resolve,0));
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'navigation:result',ok:false,code:'PLUGIN_NAVIGATION_ROUTE_MISMATCH'});
    }finally{f.close();}
  });
  it('replays the latest session to a late adapter and acknowledges successful application',async()=>{
    const f=fixture();try{
      f.message('sync');f.message('auth-token',{sequence:1,sessionId:'session',revision:1,expiresAt:Date.now()+10000,accessToken:'current'});
      const cb=vi.fn(async()=>{});f.dom.window.PowerXBridgeClient.onAuthToken(cb);
      await new Promise(resolve=>setTimeout(resolve,0));expect(cb).toHaveBeenCalledTimes(1);
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'auth:applied',sequence:1,sessionId:'session',revision:1});
    }finally{f.close();}
  });
  it('reports rejected context without applying readiness',async()=>{
    const f=fixture();try{
      f.message('sync');f.dom.window.PowerXBridgeClient.onAuthToken(async()=>{throw new Error('context failed')});
      f.message('auth-token',{sequence:1,sessionId:'session',revision:1,expiresAt:Date.now()+10000,accessToken:'current'});
      await new Promise(resolve=>setTimeout(resolve,0));expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'auth:rejected',sequence:1});
    }finally{f.close();}
  });
  it('rejects navigation outside the current plugin mount',()=>{
    const f=fixture();try{
      f.message('sync');f.message('navigate',{requestId:'wrong',path:'/_p/other/admin/private'});
      expect(f.post.mock.calls.at(-1)?.[0]).toMatchObject({type:'navigation:result',ok:false});expect(f.dom.window.location.pathname).toContain('/current');
    }finally{f.close();}
  });

});
