import { beforeEach, afterEach, expect, it, vi } from 'vitest';
const calls = vi.hoisted(() => ({ request: vi.fn(), stream: vi.fn() }));
vi.mock('h3', () => ({
  createError: (value: any) => Object.assign(new Error(value.statusMessage), value),
  defineEventHandler: (handler: any) => handler,
  getHeader: (event: any, name: string) => event.headers?.[name],
  getRequestURL: (event: any) => new URL(event.url),
  proxyRequest: calls.request,
  setResponseHeader: vi.fn(),
}));
vi.mock('../../../server/utils/backup-stream-proxy', () => ({ proxyBackupStream: calls.stream }));
import handler from '../../../server/middleware/00-recovery-progress';
const event = (path: string, method = 'GET', headers: Record<string, string> = {}) => ({ url: `http://127.0.0.1:3030${path}`, method, headers }) as any;
beforeEach(() => {
  vi.clearAllMocks();vi.stubEnv('POWERX_BACKEND', '');vi.stubEnv('POWERX_RECOVERY_STATUS_URL', '');
  vi.stubGlobal('useRuntimeConfig', () => ({ upstream: 'http://127.0.0.1:8077/api/v1' }));
});
afterEach(() => { vi.unstubAllEnvs();vi.unstubAllGlobals(); });
it('uses the original Core backend for fixed recovery UI and controls', async () => {
  for (const [path, method] of [['', 'GET'], ['app.js', 'GET'], ['catalog', 'GET'], ['session', 'POST'], ['session', 'DELETE'], ['restore', 'POST']] as const) {
    await handler(event(`/__powerx_recovery/${path}`, method));
    expect(calls.request).toHaveBeenLastCalledWith(expect.anything(), `http://127.0.0.1:8077/__powerx_recovery/${path}`, { headers: { authorization: '', host: '127.0.0.1:8077' } });
  }
});
it('streams maintenance progress through the same backend with a read-only header ticket', async () => {
  const path = '/__powerx_recovery/jobs/00000000-0000-4000-8000-000000000001/events';
  const authorization = `Bearer 1793000000.${'a'.repeat(64)}`;
  await handler(event(path, 'GET', { authorization }));
  expect(calls.stream).toHaveBeenCalledWith(expect.anything(), `http://127.0.0.1:8077${path}`, { authorization, host: '127.0.0.1:8077' });
  await expect(handler(event(path, 'POST', { authorization }))).rejects.toMatchObject({ statusCode: 400 });
  await expect(handler(event(path))).rejects.toMatchObject({ statusCode: 401 });
});
it('rejects arbitrary operations, URL credentials and tokens in URLs', async () => {
  for (const path of ['/__powerx_recovery/execute', '/__powerx_recovery/session?token=secret', '/__powerx_recovery/jobs/invalid']) await expect(handler(event(path))).rejects.toMatchObject({ statusCode: 400 });
  vi.stubEnv('POWERX_BACKEND', 'http://user:password@127.0.0.1:8077');
  await expect(handler(event('/__powerx_recovery/catalog'))).rejects.toMatchObject({ statusCode: 503 });
  expect(calls.request).not.toHaveBeenCalled();
});
it('validates browser Origin before translating it to the upstream Origin',async()=>{
  await handler(event('/__powerx_recovery/session','POST',{origin:'http://127.0.0.1:3030'}));
  expect(calls.request).toHaveBeenLastCalledWith(expect.anything(),'http://127.0.0.1:8077/__powerx_recovery/session',{headers:{authorization:'',host:'127.0.0.1:8077',origin:'http://127.0.0.1:8077'}});
  calls.request.mockClear();
  await expect(handler(event('/__powerx_recovery/restore','POST',{origin:'https://attacker.invalid'}))).rejects.toMatchObject({statusCode:403});
  await expect(handler(event('/__powerx_recovery/session','POST',{'sec-fetch-site':'cross-site'}))).rejects.toMatchObject({statusCode:403});
  expect(calls.request).not.toHaveBeenCalled();
});
