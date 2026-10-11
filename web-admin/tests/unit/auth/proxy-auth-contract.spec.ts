import { beforeEach, describe, expect, it, vi } from 'vitest';
const stubs=vi.hoisted(()=>({status:vi.fn(),header:vi.fn(),proxy:vi.fn(),cookie:vi.fn(),getHeader:vi.fn()}));
vi.mock('h3',()=>({defineEventHandler:(fn:any)=>fn,getCookie:stubs.cookie,getHeader:stubs.getHeader,
 getRequestURL:(event:any)=>new URL(event.url,'http://localhost:3030'),proxyRequest:stubs.proxy,
 setResponseHeader:stubs.header,setResponseStatus:stubs.status}));
import handler from '../../../server/middleware/00-proxy-plugins';
beforeEach(()=>{vi.clearAllMocks();vi.stubGlobal('useRuntimeConfig',()=>({upstream:'http://localhost:8077'}));stubs.getHeader.mockReturnValue('');stubs.cookie.mockReturnValue('host-token');});
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
});
