import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { ref } from 'vue';
import { commitAuthSession } from '~/utils/auth-session';
const getContext = vi.hoisted(()=>vi.fn());
vi.mock('~/composables/useMe',()=>({useMe:()=>({getUserContext:getContext})}));
vi.mock('~/composables/useAuth',()=>({useAuth:()=>({})}));
import { useUserStore } from '~/stores/user';
const start = (tid:string)=>commitAuthSession({access_token:`h.${btoa(JSON.stringify({tid,exp:Math.floor(Date.now()/1000)+3600}))}.s`,refresh_token:`refresh-${tid}`,expires_in:3600});
const deferred = ()=>{let resolve!:(value:any)=>void,reject!:(reason:any)=>void;const promise=new Promise((yes,no)=>{resolve=yes;reject=no});return{promise,resolve,reject};};
beforeEach(()=>{(process as any).client=true;localStorage.clear();setActivePinia(createPinia());getContext.mockReset();vi.stubGlobal('useState',(_key:string,init:()=>any)=>ref(init()));});
describe('user context generation',()=>{
  it('does not share an old in-flight context with a new login or commit its response',async()=>{
    start('old');const old=deferred(),current=deferred();getContext.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    const store=useUserStore(),one=store.fetchUserContext({force:true});const rejected=expect(one).rejects.toMatchObject({code:'AUTH_SESSION_CHANGED'});
    start('current');const two=store.fetchUserContext({force:true});expect(getContext).toHaveBeenCalledTimes(2);
    old.resolve({current_tenant_uuid:'old'});await rejected;expect(store.isLoading).toBe(true);
    current.resolve({current_tenant_uuid:'current'});await two;expect(store.currentTenantUuid).toBe('current');expect(store.isLoading).toBe(false);
  });
  it('does not let an old context error clear a newer context or error state',async()=>{
    start('old');const old=deferred();getContext.mockReturnValueOnce(old.promise);
    const store=useUserStore(),one=store.fetchUserContext({force:true});const rejected=expect(one).rejects.toMatchObject({code:'AUTH_SESSION_CHANGED'});
    start('current');getContext.mockResolvedValueOnce({current_tenant_uuid:'current'});await store.fetchUserContext({force:true});
    old.reject({status:401,message:'old failure'});await rejected;expect(store.currentTenantUuid).toBe('current');expect(store.error).toBeNull();
  });
  it('deduplicates only requests from the same current session',async()=>{
    start('current');const reply=deferred();getContext.mockReturnValue(reply.promise);const store=useUserStore();
    const one=store.fetchUserContext({force:true}),two=store.fetchUserContext({force:true});expect(getContext).toHaveBeenCalledTimes(1);
    reply.resolve({current_tenant_uuid:'current'});await Promise.all([one,two]);expect(store.currentTenantUuid).toBe('current');
  });
});
