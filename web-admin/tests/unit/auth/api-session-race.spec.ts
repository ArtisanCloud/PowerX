import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
const loading=vi.hoisted(()=>({pending:{value:0},auto:{value:false},nav:{value:false},progress:vi.fn(),message:vi.fn()}));
vi.mock('~/composables/useGlobalLoading',()=>({useGL_ReqPending:()=>loading.pending,useGL_AutoVisible:()=>loading.auto,useGL_NavPending:()=>loading.nav,useGlobalLoading:()=>({setProgress:loading.progress,setMessage:loading.message})}));
import { setApiConfig, useApiClient } from '~/composables/api';
import { commitAuthSession, readAuthSession } from '~/utils/auth-session';
const jwt=(tid:string)=>`h.${btoa(JSON.stringify({tid,exp:Math.floor(Date.now()/1000)+3600}))}.s`;
const start=(tid:string)=>commitAuthSession({access_token:jwt(tid),refresh_token:`refresh-${tid}`,expires_in:3600});
const deferred=()=>{let resolve!:(value:any)=>void,reject!:(error:any)=>void;const promise=new Promise((yes,no)=>{resolve=yes;reject=no});return{promise,resolve,reject};};
beforeEach(()=>{
 (process as any).client=true;localStorage.clear();loading.pending.value=0;loading.auto.value=false;vi.clearAllMocks();
 setApiConfig({baseURL:'/api/v1'});vi.stubGlobal('useState',(_key:string,init:()=>any)=>ref(init()));
 Object.defineProperty(navigator,'locks',{configurable:true,value:{request:(_name:string,fn:()=>any)=>fn()}});
});
describe('API session transaction',()=>{
 it('reads the host snapshot despite a stale scalar mirror',async()=>{
   const current=start('current');localStorage.setItem('access_token','old-plugin');const fetch=vi.fn().mockResolvedValue({ok:true});vi.stubGlobal('$fetch',fetch);
   await useApiClient().get('/admin/user/auth/me/context');expect(fetch.mock.calls[0][1].headers.Authorization).toBe(`Bearer ${current.accessToken}`);expect(loading.pending.value).toBe(0);
 });
 it('cancels a delayed success and releases its loading state after another login',async()=>{
   start('old');const reply=deferred(),fetch=vi.fn(()=>reply.promise);vi.stubGlobal('$fetch',fetch);
   const result=useApiClient().get('/admin/user/auth/me/context'),assertion=expect(result).rejects.toMatchObject({code:'AUTH_SESSION_CHANGED'});
   await vi.waitFor(()=>expect(fetch).toHaveBeenCalled());const current=start('current');reply.resolve({tenant:'old'});await assertion;
   expect(readAuthSession()).toEqual(current);expect(loading.pending.value).toBe(0);expect(loading.auto.value).toBe(false);
 });
 it('does not refresh a delayed old 401 using the new login',async()=>{
   start('old');const reply=deferred(),fetch=vi.fn(()=>reply.promise);vi.stubGlobal('$fetch',fetch);
   const result=useApiClient().get('/admin/user/auth/me/context'),assertion=expect(result).rejects.toMatchObject({code:'AUTH_SESSION_CHANGED'});
   await vi.waitFor(()=>expect(fetch).toHaveBeenCalled());start('current');reply.reject({status:401,response:{status:401}});await assertion;
   expect(fetch).toHaveBeenCalledTimes(1);expect(loading.pending.value).toBe(0);
 });
 it('refreshes once on current 401 and balances loading across the retried request',async()=>{
   start('current');const fetch=vi.fn().mockRejectedValueOnce({status:401,response:{status:401}})
    .mockResolvedValueOnce({data:{access_token:jwt('current'),refresh_token:'rotated',expires_in:3600}}).mockResolvedValueOnce({ok:true});vi.stubGlobal('$fetch',fetch);
   expect(await useApiClient().get('/admin/user/auth/me/context')).toEqual({ok:true});
   expect(fetch).toHaveBeenCalledTimes(3);expect(fetch.mock.calls[1][0]).toBe('/api/v1/admin/user/auth/refresh');
   expect(readAuthSession()?.refreshToken).toBe('rotated');expect(loading.pending.value).toBe(0);expect(loading.auto.value).toBe(false);
 });
 it('cancels a request when login changes during an async request interceptor',async()=>{
   start('old');const gate=deferred(),entered=vi.fn();let active=true;
   setApiConfig({requestInterceptors:[{onRequest:async config=>{if(active){entered();await gate.promise}return config}}]});
   const fetch=vi.fn();vi.stubGlobal('$fetch',fetch);const result=useApiClient().post('/customer/update',{name:'old-context'});
   const assertion=expect(result).rejects.toMatchObject({code:'AUTH_SESSION_CHANGED'});
   await vi.waitFor(()=>expect(entered).toHaveBeenCalled());start('current');active=false;gate.resolve(undefined);await assertion;
   expect(fetch).not.toHaveBeenCalled();expect(loading.pending.value).toBe(0);
 });

});
