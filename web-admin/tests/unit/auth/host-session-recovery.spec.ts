import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthSessionChangedError, captureAuthSession, clearAuthSession, commitAuthSession, readAuthSession, refreshAuthSession } from '~/utils/auth-session';
import { createHostSessionRecovery } from '~/utils/host-session-recovery';
import { loginRecoveryLocation } from '~/utils/auth-failure';
const login = (tenant = 'current') => commitAuthSession({access_token:`h.${btoa(JSON.stringify({tid:tenant,exp:Math.floor(Date.now()/1000)+3600}))}.s`,refresh_token:`refresh-${tenant}`});
const terminal = (code = 'AUTH_SESSION_EXPIRED') => ({status:401,data:{error_code:code}});
const deferred = () => { let resolve!:(value?:unknown)=>void,reject!:(error:unknown)=>void;const promise=new Promise((yes,no)=>{resolve=yes;reject=no});return{promise,resolve,reject}; };
beforeEach(()=>{(process as any).client=true;localStorage.clear();Object.defineProperty(navigator,'locks',{configurable:true,value:{request:(_key:string,callback:()=>unknown)=>callback()}});});
describe('host verifies session termination',()=>{
  it('keeps a valid host session and coalesces simultaneous plugin probes',async()=>{
    const snapshot=login(),check=deferred(),probe=vi.fn(()=>check.promise),redirect=vi.fn(),refresh=vi.fn();
    const recover=createHostSessionRecovery({probe,refresh,login:redirect});
    const one=recover(captureAuthSession()!),two=recover(captureAuthSession()!);await Promise.resolve();expect(probe).toHaveBeenCalledTimes(1);
    check.resolve({status:'active'});expect(await one).toBe('active');expect(await two).toBe('active');
    expect(readAuthSession()).toEqual(snapshot);expect(refresh).not.toHaveBeenCalled();expect(redirect).not.toHaveBeenCalled();
  });
  it.each(['AUTH_SESSION_EXPIRED','AUTH_SESSION_REVOKED','AUTH_SESSION_INVALID'])('clears only after Core confirms %s',async code=>{
    login();const redirect=vi.fn(),recover=createHostSessionRecovery({probe:async()=>{throw terminal(code)},refresh:vi.fn(),login:redirect});
    expect(await recover(captureAuthSession()!)).toBe('signed-out');expect(readAuthSession()?.status).toBe('signed-out');expect(redirect).toHaveBeenCalledTimes(1);
  });
  it.each([{status:503},{status:403,data:{error_code:'AUTH_USER_FORBIDDEN'}},{status:401},{status:401,data:{error_code:'IAM_CONTEXT_SERVICE_CREDENTIAL_EXPIRED'}}])('preserves the host on ambiguous, service and temporary errors %j',async error=>{
    const snapshot=login(),redirect=vi.fn(),recover=createHostSessionRecovery({probe:async()=>{throw error},refresh:vi.fn(),login:redirect});
    await expect(recover(captureAuthSession()!)).rejects.toEqual(error);expect(readAuthSession()).toEqual(snapshot);expect(redirect).not.toHaveBeenCalled();
  });
  it('ignores a late terminal answer after another login or same-session refresh',async()=>{
    for(const change of ['login','refresh']){
      login('old');const stamp=captureAuthSession()!,check=deferred(),redirect=vi.fn(),recover=createHostSessionRecovery({probe:()=>check.promise,refresh:vi.fn(),login:redirect});
      const result=recover(stamp);const assertion=expect(result).rejects.toBeInstanceOf(AuthSessionChangedError);await Promise.resolve();
      const next=change==='login'?login('new'):commitAuthSession({access_token:readAuthSession()!.accessToken},stamp);
      check.reject(terminal());await assertion;expect(readAuthSession()).toEqual(next);expect(redirect).not.toHaveBeenCalled();
    }
  });
  it('ignores a terminal answer after the reporting frame is removed',async()=>{
    const snapshot=login(),check=deferred(),redirect=vi.fn(),controller=new AbortController();
    const recover=createHostSessionRecovery({probe:()=>check.promise,refresh:vi.fn(),login:redirect});const result=recover(captureAuthSession()!,controller.signal);
    const assertion=expect(result).rejects.toBeInstanceOf(AuthSessionChangedError);await Promise.resolve();controller.abort();check.reject(terminal());await assertion;
    expect(readAuthSession()).toEqual(snapshot);expect(redirect).not.toHaveBeenCalled();
  });
  it('joins expired-host renewal and redirects once on a verified refresh rejection',async()=>{
    login();localStorage.setItem('powerx.session',JSON.stringify({...readAuthSession(),expiresAt:Date.now()-1}));
    const redirect=vi.fn(),probe=vi.fn(),request=vi.fn(async()=>{throw terminal()}),recover=createHostSessionRecovery({probe,refresh:()=>refreshAuthSession(request),login:redirect});
    expect(await recover(captureAuthSession()!)).toBe('signed-out');expect(request).toHaveBeenCalledTimes(1);expect(probe).not.toHaveBeenCalled();expect(redirect).toHaveBeenCalledTimes(1);
  });
  it('does not navigate when a new login occurs during signed-out cleanup',async()=>{
    login();const redirect=vi.fn(),onChange=()=>{if(readAuthSession()?.status==='signed-out')login('new')};window.addEventListener('powerx:auth-session-changed',onChange);
    try{const recover=createHostSessionRecovery({probe:async()=>{throw terminal()},refresh:vi.fn(),login:redirect});await expect(recover(captureAuthSession()!)).rejects.toBeInstanceOf(AuthSessionChangedError);expect(readAuthSession()?.tenantUuid).toBe('new');expect(redirect).not.toHaveBeenCalled();}
    finally{window.removeEventListener('powerx:auth-session-changed',onChange)}
  });
  it('preserves path, query and hash without accepting an external redirect',()=>{
    const path='/_p/com.powerx.plugins.ai-craft/admin/tracks?tab=mine#draft';expect(loginRecoveryLocation(path)).toBe(`/users/login?redirect=${encodeURIComponent(path)}`);
    for(const external of ['https://other.invalid','//other.invalid','/\\other.invalid','javascript:alert(1)'])expect(loginRecoveryLocation(external)).toBe('/users/login?redirect=%2Fhome');
  });
});

it('renews once when Core explicitly reports access expiry despite the local clock',async()=>{
  login();const first=captureAuthSession()!;const probe=vi.fn().mockRejectedValueOnce({status:401,data:{error_code:'AUTH_ACCESS_EXPIRED'}}).mockResolvedValueOnce({status:'active'});
  const refresh=vi.fn(async()=>commitAuthSession({access_token:readAuthSession()!.accessToken},captureAuthSession()!)),redirect=vi.fn();
  const recover=createHostSessionRecovery({probe,refresh,login:redirect});expect(await recover(first)).toBe('active');expect(probe).toHaveBeenCalledTimes(2);expect(refresh).toHaveBeenCalledTimes(1);expect(redirect).not.toHaveBeenCalled();
});

it('cancels an unneeded probe but does not cancel a probe still used by another frame',async()=>{
  login();const check=deferred(),controllers:AbortSignal[]=[],probe=vi.fn((_snapshot,signal)=>{controllers.push(signal);return check.promise});
  const recover=createHostSessionRecovery({probe,refresh:vi.fn(),login:vi.fn()}),one=new AbortController(),two=new AbortController(),stamp=captureAuthSession()!;
  const a=recover(stamp,one.signal),b=recover(stamp,two.signal);const rejected=expect(a).rejects.toBeInstanceOf(AuthSessionChangedError);await Promise.resolve();one.abort();expect(controllers[0].aborted).toBe(false);
  check.resolve({status:'active'});await rejected;expect(await b).toBe('active');expect(probe).toHaveBeenCalledTimes(1);expect(controllers[0].aborted).toBe(true);
});
