import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { delimiter, resolve } from 'node:path';

const webPort = Number(process.env.POWERX_SESSION_TEST_PORT || 3473);
const backendPort = Number(process.env.POWERX_SESSION_TEST_BACKEND_PORT || 3474);
if (![webPort, backendPort].every(port => Number.isInteger(port) && port > 1024 && port < 65536) || webPort === backendPort) {
  throw new Error('Session browser tests require two distinct unprivileged ports');
}
const entry = resolve('.output/server/index.mjs');
if (!existsSync(entry)) throw new Error('Run npm run build before the session browser tests');

const pluginId = 'com.powerx.plugins.ai-craft';
const tenantUuid = 'session-browser-fixture';
const token = () => `header.${Buffer.from(JSON.stringify({ tid: tenantUuid, exp: Math.floor(Date.now() / 1000) + 3600 })).toString('base64url')}.fixture`;
const requests = [];
let rejectDocuments = false;
const context = {
  is_root: true, current_tenant_uuid: tenantUuid, current_member_id: 1,
  user: { id: 1, email: 'fixture@example.invalid', phone: '', display_name: 'Session fixture', avatar_url: '', status: 1, is_root: true },
  members: [{ tenant_uuid: tenantUuid, tenant_name: 'Session fixture', member_id: 1, is_admin: true }],
};
const pluginHtml = `<!doctype html><html><head><meta charset="utf-8"></head><body>
<main data-testid="plugin-ready">Session fixture</main>
<script>
addEventListener('DOMContentLoaded', function () {
  var client = window.PowerXBridgeClient;
  if (!client || client.protocolVersion !== 2) throw new Error('Missing injected v2 client');
  client.setNavigator(function (path, request) {
    if (request.signal.aborted) throw new Error('Cancelled navigation');
    history.pushState(null, '', path);
  });
  client.onAuthToken(async function (payload) {
    if (payload.type === 'auth-cleared') { document.body.dataset.auth = 'cleared'; return; }
    var response = await fetch('/api/v1/admin/user/auth/me/context', {
      headers: { Authorization: 'Bearer ' + payload.accessToken }
    });
    if (!response.ok) throw new Error('Context verification failed');
    await response.json();
    document.body.dataset.auth = 'applied';
  });
});
</script></body></html>`;

function json(response, status, data) {
  response.writeHead(status, { 'content-type': 'application/json', 'cache-control': 'no-store' });
  response.end(JSON.stringify(data));
}
const backend = createServer(async (request, response) => {
  const url = new URL(request.url || '/', `http://127.0.0.1:${backendPort}`);
  if (url.pathname === '/api/v1/__session_test/state') {
    if (request.method === 'DELETE') { requests.length = 0; rejectDocuments = false; }
    if (request.method === 'PUT') {
      let body = '';
      for await (const chunk of request) body += chunk;
      rejectDocuments = JSON.parse(body).rejectDocuments === true;
    }
    json(response, 200, { requests });
    return;
  }
  const authorization = String(request.headers.authorization || '');
  const hasBearer = authorization.startsWith('Bearer ');
  requests.push({ method: request.method, path: url.pathname, query: url.search, hasBearer, userAgent: request.headers['user-agent'] || '' });
  if (url.pathname.startsWith(`/_p/${pluginId}/admin`)) {
    if (rejectDocuments || !hasBearer) { json(response, 401, { error: 'missing or invalid Authorization header' }); return; }
    response.writeHead(200, { 'content-type': 'text/html', 'cache-control': 'no-store' });
    response.end(pluginHtml);
  } else if (url.pathname.endsWith('/setup/status')) {
    json(response, 200, { data: { configured: true, requires_login: true, restart_required: false, install_status: 'installed' } });
  } else if (url.pathname.endsWith('/user/auth/refresh')) {
    json(response, 200, { data: { access_token: token(), refresh_token: 'fixture-refresh-renewed', expires_in: 3600, token_type: 'Bearer', scope: 'access' } });
  } else if (url.pathname.endsWith('/user/auth/me/context')) {
    json(response, hasBearer ? 200 : 401, hasBearer ? { data: context } : { error: 'missing bearer' });
  } else if (url.pathname.endsWith('/admin/menus')) {
    json(response, 200, { data: { categories: [{ id: 'fixture', title: 'Session fixture', order: 1, origin: 'plugin', children: [
      { id: `plugin:${pluginId}:tracks`, title: 'Tracks', path: '/operations/ai-craft/tracks', origin: 'plugin', visible: true },
    ] }] } });
  } else {
    json(response, 200, { data: { items: [], total: 0 } });
  }
});
await new Promise((yes, no) => { backend.once('error', no); backend.listen(backendPort, '127.0.0.1', yes); });

const child = spawn(process.execPath, [entry], {
  cwd: process.cwd(), stdio: ['ignore', 'inherit', 'inherit'],
  env: {
    ...process.env, NODE_ENV: 'production', HOST: '127.0.0.1', PORT: String(webPort),
    POWERX_BACKEND: `http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_API_BASE: '/api/v1',
    NUXT_PUBLIC_POWERX_CORE_BASE: `http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_UPSTREAM_ORIGIN: `http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_WS_ORIGIN: `ws://127.0.0.1:${backendPort}`,
    NODE_PATH: [resolve('node_modules'), process.env.NODE_PATH].filter(Boolean).join(delimiter),
  },
});
const record = {
  owner: 'session-browser-tests', cwd: process.cwd(), startedAt: new Date().toISOString(),
  fixturePid: process.pid, fixturePort: backendPort, corePid: child.pid, corePort: webPort,
  command: [process.execPath, entry],
};
const resultsDir = resolve('../test-results');
mkdirSync(resultsDir, { recursive: true });
function saveRecord() {
  writeFileSync(resolve(resultsDir, 'session-browser-processes.json'), JSON.stringify(record, null, 2), { mode: 0o600 });
}
saveRecord();
console.log(JSON.stringify(record));
let stopping = false;
function shutdown(signal) {
  if (stopping) return;
  stopping = true;
  record.stoppedAt = new Date().toISOString();
  saveRecord();
  backend.close();
  backend.closeAllConnections();
  if (child.exitCode !== null || child.signalCode !== null) { process.exitCode = child.exitCode || 0; return; }
  child.kill(signal);
  const force = setTimeout(() => child.kill('SIGKILL'), 5000);
  force.unref();
  child.once('exit', () => clearTimeout(force));
}
process.once('SIGTERM', () => shutdown('SIGTERM'));
process.once('SIGINT', () => shutdown('SIGINT'));
child.once('error', error => { console.error(error); shutdown('SIGTERM'); process.exitCode = 1; });
child.once('exit', code => {
  record.coreExitedAt = new Date().toISOString();
  saveRecord();
  if (!stopping) { shutdown('SIGTERM'); process.exitCode = code || 1; }
});
