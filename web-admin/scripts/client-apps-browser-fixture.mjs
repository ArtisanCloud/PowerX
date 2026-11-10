// Controlled browser fixture. Never connects to the user's Core or business database.
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { resolve, delimiter } from 'node:path';

const port = Number(process.env.POWERX_CLIENT_APPS_TEST_PORT || 3543);
const backendPort = Number(process.env.POWERX_CLIENT_APPS_TEST_BACKEND_PORT || 3544);
const output = resolve(process.env.POWERX_WEB_OUTPUT || '.output');
const artifacts = resolve(process.env.POWERX_CLIENT_APPS_ARTIFACTS || '../test-results/client-apps');
if (!existsSync(resolve(output, 'server/index.mjs'))) throw new Error('Build an isolated web output first');
mkdirSync(artifacts, { recursive: true });
const tenant = '11111111-1111-4111-8111-111111111111';
let apps = [], sequence = 0, conflict = false, unavailable = false;
const requests = [];
const json = (res, status, data) => { res.writeHead(status, { 'content-type': 'application/json', 'cache-control': 'no-store' }); res.end(JSON.stringify(data)); };
const backend = createServer(async (req, res) => {
  const url = new URL(req.url, `http://127.0.0.1:${backendPort}`);
  let raw = ''; for await (const chunk of req) raw += chunk;
  const body = raw ? JSON.parse(raw) : {};
  if (url.pathname.endsWith('/__client_apps_test/state')) {
    if (req.method === 'DELETE') { apps = []; requests.length = 0; conflict = false; unavailable = false; }
    if (req.method === 'PUT') { conflict = body.conflict === true; unavailable = body.unavailable === true; }
    return json(res, 200, { apps, requests });
  }
  requests.push({ method: req.method, path: url.pathname, query: url.search, body });
  if (url.pathname.endsWith('/setup/status')) return json(res, 200, { data: { configured: true, requires_login: true, restart_required: false, install_status: 'installed' } });
  if (url.pathname.endsWith('/user/auth/me/session')) return json(res, 200, { data: { status: 'active', session_expires_at: Date.now() + 3600000 } });
  if (url.pathname.endsWith('/user/auth/me/context')) return json(res, 200, { data: { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, email: 'fixture@example.invalid', display_name: 'Fixture admin', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '应用验收租户', member_id: 1, is_admin: true }] } });
  if (url.pathname.endsWith('/admin/menus')) return json(res, 200, { data: { categories: [{ id: 'settings', title: 'menu.section.settings', order: 10, origin: 'system', children: [{ id: 'settings', title: 'menu.settings', path: '/settings', visible: true, origin: 'system', children: [{ id: 'settings_client_apps', title: 'menu.clientApps', path: '/settings/client-apps', visible: true, origin: 'system' }] }] }] } });
  if (url.pathname.includes('/customers/client-apps')) {
    if (unavailable) return json(res, 503, { error_code: 'CLIENT_APP_STORAGE_UNAVAILABLE' });
    if (url.pathname.endsWith('/catalog')) return json(res, 200, { data: { payload: { kinds: [{ id: 'mini_program', supported: true }, { id: 'mobile_app', supported: false }, { id: 'web_app', supported: false }], providers: [{ id: 'wechat', supported: true }, { id: 'alipay', supported: false }, { id: 'douyin', supported: false }], plugin_ids: ['com.example.shop'], legacy_applications: [] } } });
    const id = url.pathname.split('/client-apps/')[1];
    if (req.method === 'POST' || req.method === 'PATCH') {
      const previous = apps.find(a => a.uuid === id);
      if (conflict || previous && previous.version !== body.expected_version) return json(res, 409, { error_code: 'CLIENT_APP_VERSION_CONFLICT' });
      const app = { uuid: previous?.uuid || `app-${++sequence}`, name: body.name, kind: body.kind, status: body.status, version: (previous?.version || 0) + 1, plugin_ids: body.plugin_ids, updated_at: new Date().toISOString(), connections: body.connections.map(c => ({ uuid: previous?.connections[0]?.uuid || `binding-${sequence}`, provider: c.provider, app_id: c.app_id, platform: 'wechat_miniapp', secret_configured: !!c.app_secret || !!previous?.connections[0]?.secret_configured })) };
      apps = [...apps.filter(a => a.uuid !== app.uuid), app];
      return json(res, 200, { data: { payload: app } });
    }
    if (id) return json(res, 200, { data: { payload: apps.find(a => a.uuid === id) } });
    const items = apps.filter(a => (!url.searchParams.get('kind') || a.kind === url.searchParams.get('kind')) && (!url.searchParams.get('provider') || a.connections.some(c => c.provider === url.searchParams.get('provider'))) && a.name.includes(url.searchParams.get('q') || ''));
    return json(res, 200, { data: { payload: { items, pagination: { total: items.length, page: 1, page_size: 20 } } } });
  }
  return json(res, 200, { data: { items: [], total: 0, allowed: true, has_permission: true } });
});
await new Promise((yes, no) => { backend.once('error', no); backend.listen(backendPort, '127.0.0.1', yes); });
const child = spawn(process.execPath, [resolve(output, 'server/index.mjs')], { cwd: process.cwd(), stdio: 'inherit', env: { ...process.env, NODE_ENV: 'production', HOST: '127.0.0.1', PORT: String(port), POWERX_BACKEND: `http://127.0.0.1:${backendPort}`, NUXT_PUBLIC_API_BASE: '/api/v1', NUXT_PUBLIC_POWERX_CORE_BASE: `http://127.0.0.1:${backendPort}`, NUXT_PUBLIC_UPSTREAM_ORIGIN: `http://127.0.0.1:${backendPort}`, NUXT_PUBLIC_WS_ORIGIN: `ws://127.0.0.1:${backendPort}`, NODE_PATH: [resolve('node_modules'), process.env.NODE_PATH].filter(Boolean).join(delimiter) } });
const record = { owner: 'client-apps-browser-tests', cwd: process.cwd(), fixturePid: process.pid, fixturePort: backendPort, webPid: child.pid, webPort: port, command: [process.execPath, resolve(output, 'server/index.mjs')] };
writeFileSync(resolve(artifacts, 'processes.json'), JSON.stringify(record, null, 2), { mode: 0o600 });
let stopping = false;
function stop() { if (stopping) return; stopping = true; child.kill('SIGTERM'); backend.close(); }
process.on('SIGTERM', stop); process.on('SIGINT', stop);
child.on('exit', code => { backend.close(); process.exitCode = stopping ? 0 : code || 0; });
