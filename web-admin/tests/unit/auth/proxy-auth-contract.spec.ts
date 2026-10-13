import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
const stubs=vi.hoisted(()=>({status:vi.fn(),header:vi.fn(),proxy:vi.fn(),cookie:vi.fn(),getHeader:vi.fn()}));
vi.mock('h3',()=>({defineEventHandler:(fn:any)=>fn,getCookie:stubs.cookie,getHeader:stubs.getHeader,
 getRequestURL:(event:any)=>new URL(event.url,'http://localhost:3030'),proxyRequest:stubs.proxy,
 setResponseHeader:stubs.header,setResponseStatus:stubs.status}));
import handler from '../../../server/middleware/00-proxy-plugins';
beforeEach(()=>{vi.clearAllMocks();vi.stubGlobal('useRuntimeConfig',()=>({upstream:'http://localhost:8077'}));stubs.getHeader.mockReturnValue('');stubs.cookie.mockReturnValue('host-token');});
afterEach(()=>vi.unstubAllGlobals());
describe('host session proxy boundary',()=>{
  it.each(['login','refresh','logout'])('rejects embedded %s without an upstream call or Cookie write',async(operation)=>{
    const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
    const result=await (handler as any)({url:`/_p/plugin/api/v1/admin/user/auth/${operation}`,node:{req:{headers:{}}}});
    expect(result).toMatchObject({code:409,error_code:'AUTH_SESSION_HOST_MANAGED'});
    expect(stubs.status).toHaveBeenCalledWith(expect.anything(),409);expect(fetch).not.toHaveBeenCalled();expect(stubs.proxy).not.toHaveBeenCalled();
    expect(stubs.header.mock.calls.some(args=>String(args[1]).toLowerCase()==='set-cookie')).toBe(false);
  });
  it('keeps context read on the checked Core user route',async()=>{
    await (handler as any)({url:'/_p/plugin/api/v1/admin/user/auth/me/context',node:{req:{headers:{}}}});
    expect(stubs.proxy).toHaveBeenCalledWith(expect.anything(),'http://localhost:8077/api/v1/admin/user/auth/me/context');
  });
  it('leaves the normal host login route to its existing handler',async()=>{
    expect(await (handler as any)({url:'/api/v1/admin/user/auth/login'})).toBeUndefined();expect(stubs.status).not.toHaveBeenCalled();
  });
  it('preserves document 401 while exposing a fixed load marker without upstream error details',async()=>{
    stubs.getHeader.mockImplementation((_event:any,name:string)=>name==='accept'?'text/html':'');
    stubs.cookie.mockReturnValue('');
    const fetch=vi.fn().mockResolvedValue(new Response(JSON.stringify({error:'missing or invalid Authorization header',detail:'private-upstream-detail'}),{
      status:401,headers:{'content-type':'application/json'},
    }));
    vi.stubGlobal('fetch',fetch);
    const event={url:'/_p/plugin/admin/current?__px_iframe=1',method:'GET',node:{req:{headers:{}}}};
    const result=await (handler as any)(event);
    expect(stubs.status).toHaveBeenCalledWith(event,401);
    expect(stubs.header).toHaveBeenCalledWith(event,'x-px-auth-injected','0');
    expect(stubs.header).toHaveBeenCalledWith(event,'cache-control','no-store');
    expect(result).toContain('data-powerx-plugin-frame-error="PLUGIN_FRAME_AUTH_REQUIRED"');
    expect(result).not.toContain('private-upstream-detail');
    expect(result).not.toContain('powerx-bridge-client.js');
    expect(stubs.proxy).not.toHaveBeenCalled();
  });
  it('continues injecting the v2 bridge into successful iframe documents',async()=>{
    stubs.getHeader.mockImplementation((_event:any,name:string)=>name==='accept'?'text/html':'');
    vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response('<html><head></head><body>plugin</body></html>',{
      status:200,headers:{'content-type':'text/html'},
    })));
    const result=await (handler as any)({url:'/_p/plugin/admin/current?__px_iframe=1',method:'GET',node:{req:{headers:{}}}});
    expect(result).toContain('<script src="/powerx-bridge-client.js?v=2"></script>');
    expect(result).not.toContain('data-powerx-plugin-frame-error');
  });
  it('keeps the upstream-not-ready response available for bounded startup retries',async()=>{
    stubs.getHeader.mockImplementation((_event:any,name:string)=>name==='accept'?'text/html':'');
    vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response('PLUGIN_ADMIN_UPSTREAM_NOT_READY',{
      status:503,headers:{'content-type':'application/json'},
    })));
    const result=await (handler as any)({url:'/_p/plugin/admin/current?__px_iframe=1',method:'GET',node:{req:{headers:{}}}});
    expect(result).toBe('PLUGIN_ADMIN_UPSTREAM_NOT_READY');
  });
});
