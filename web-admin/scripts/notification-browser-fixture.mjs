import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { WebSocketServer } from 'ws';

// Isolated browser fixtures only. Real persistence/member WS delivery is tested in Go.
const webPort = Number(process.env.POWERX_NOTIFICATION_BROWSER_PORT || 3491);
const backendPort = Number(process.env.POWERX_NOTIFICATION_BACKEND_PORT || 3492);
if (![webPort, backendPort].every(p => Number.isInteger(p) && p > 1024 && p < 65536) || webPort === backendPort) throw new Error('Two distinct unprivileged fixture ports are required');
const tenant = '11111111-1111-4111-8111-111111111111';
const member = '22222222-2222-4222-8222-222222222222';
const notificationID = '33333333-3333-4333-8333-333333333333';
const taskNotificationID = '44444444-4444-4444-8444-444444444444';
const context = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, current_member_uuid: member,
  user: { id: 1, email: 'notification@example.invalid', display_name: '通知验收', status: 1, is_root: true },
  members: [{tenant_uuid: tenant, tenant_name: '通知验收', member_id: 1, member_uuid: member, is_admin: true}],
};
const initial = () => [
  { id: notificationID, title: '数据库备份完成', content: '备份任务 #803 已完成，可查看备份文件及执行记录。', relatedId: '803', relatedType: 'backup_job' },
  { id: taskNotificationID, title: '恢复验证失败', content: '恢复验证 #99 失败，请查看验证报告。', relatedId: '99', relatedType: 'restore_drill',
    actions: [{id: 'view_business', label: '查看恢复验证', type: 'primary', action: 'navigate', params: {path: '/ops/backup?drill_id=99'}}] },
].map(n => ({ ...n, type: n.id === taskNotificationID ? 'error' : 'success', category: 'system', isRead: false, isImportant: false, createdAt: '2026-10-19T04:00:00Z', updatedAt: '2026-10-19T04:00:00Z', userId: member }));
let items = initial();
const requests = [];
const streams = new Set();
const json = (response, status, data) => { response.writeHead(status, {'content-type': 'application/json', 'cache-control': 'no-store'}); response.end(JSON.stringify(data)); };
const sockets = new WebSocketServer({ noServer: true });
const publish = payload => {
  for (const ws of sockets.clients) if (ws.readyState === 1 && ws.subscribed) ws.send(JSON.stringify({type: 'event', topic: '_topic.system.notification', payload}));
};
const backend = createServer(async (request, response) => {
  const url = new URL(request.url, `http://127.0.0.1:${backendPort}`);
  if (url.pathname === '/api/v1/__notification_fixture') {
    if (request.method === 'DELETE') { items = initial(); requests.length = 0; }
    if (request.method === 'POST') publish({title: '不应进入铃铛的临时任务', type: 'info', category: 'system'});
    return json(response, 200, {requests, items, subscribers: [...sockets.clients].filter(s => s.subscribed).length});
  }
  requests.push({method: request.method, path: url.pathname, query: url.search});
  if (url.pathname.endsWith('/setup/status')) return json(response, 200, {data: {configured: true, requires_login: true, install_status: 'installed'}});
  if (url.pathname.endsWith('/user/auth/me/context')) return json(response, 200, {data: context});
  if (url.pathname.endsWith('/admin/menus')) return json(response, 200, {data: {categories: [{id: 'ops', title: '运维', children: [{id: 'backup', title: '备份中心', path: '/ops/backup', visible: true, origin: 'core'}]}]}});
  if (url.pathname === '/api/v1/admin/ops/backup/events') {
    response.writeHead(200, {'content-type': 'text/event-stream', 'cache-control': 'no-store'});
    response.write('event: ready\ndata: {}\n\n');
    streams.add(response); request.once('close', () => streams.delete(response)); return;
  }
  if (url.pathname === '/api/v1/admin/notifications') {
    const filtered = url.searchParams.has('is_read') ? items.filter(n => n.isRead === (url.searchParams.get('is_read') === 'true')) : items;
    return json(response, 200, {data: {items: filtered, pagination: {total: filtered.length, page: 1, page_size: 20}}});
  }
  const notificationPath = url.pathname.match(/^\/api\/v1\/admin\/notifications\/([^/]+)(\/read)?$/);
  if (notificationPath) {
    const item = items.find(n => n.id === notificationPath[1]);
    if (!item) return json(response, 404, {message: '通知不存在'});
    if (request.method === 'PATCH' && notificationPath[2]) {
      item.isRead = true;
      publish({eventType: 'notification_read', payload_version: 1, contract: 'notification_read_receipt.v1', tenant_uuid: tenant, userId: member, notification_uuids: [item.id]});
      return json(response, 200, {data: {ok: true}});
    }
    return json(response, 200, {data: item});
  }
  if (url.pathname === '/api/v1/admin/ops/backup/jobs/803') return json(response, 200, {data: {job: {id: 803, status: 'failed', trigger_type: 'manual', operator: 'root', error_message: '历史任务失败原因（浏览器验收）'}}});
  if (url.pathname === '/api/v1/admin/ops/backup/restore-drills/99') return json(response, 200, {data: {drill: {id: 99, source_job_id: 803, status: 'failed', report_uri: '隔离库验证报告（浏览器验收）', rto_seconds: 1}}});
  if (url.pathname.endsWith('/admin/monitor/backup/overview')) return json(response, 200, {data: {overview: {policies_enabled: 0}, runtime: {ready: true, problems: [], source_database: 'fixture'}}});
  return json(response, 200, {data: {items: [], pagination: {total: 0}, policies: [], snapshots: [], jobs: [], sources: [], alerts: [], restores: [], configured: false}});
});
backend.on('upgrade', (request, socket, head) => sockets.handleUpgrade(request, socket, head, ws => sockets.emit('connection', ws, request)));
sockets.on('connection', ws => {
  ws.send(JSON.stringify({type: 'welcome', payload: {connection_id: 'browser-fixture'}}));
  ws.on('message', raw => {
    const cmd = JSON.parse(raw.toString());
    if (cmd.type === 'subscribe') { ws.subscribed = true; ws.send(JSON.stringify({type: 'ack', payload: {topic: cmd.topic}})); }
    if (cmd.type === 'unsubscribe') ws.subscribed = false;
  });
});
await new Promise((yes, no) => { backend.once('error', no); backend.listen(backendPort, '127.0.0.1', yes); });
const entry = resolve('.output-notification-validation/server/index.mjs');
const child = spawn(process.execPath, [entry], {cwd: process.cwd(), stdio: 'inherit', env: {...process.env,
  NODE_ENV: 'production', HOST: '127.0.0.1', PORT: String(webPort), POWERX_BACKEND: `http://127.0.0.1:${backendPort}`,
  NUXT_PUBLIC_API_BASE: '/api/v1', NUXT_PUBLIC_POWERX_CORE_BASE: `http://127.0.0.1:${backendPort}`,
  NUXT_PUBLIC_UPSTREAM_ORIGIN: `http://127.0.0.1:${backendPort}`, NUXT_PUBLIC_WS_ORIGIN: `ws://127.0.0.1:${backendPort}`,
  NUXT_PUBLIC_WS_PATH: `ws://127.0.0.1:${backendPort}/api/ws`,
  NODE_PATH: resolve('node_modules'),
}});
mkdirSync('../test-results/notifications-browser', {recursive: true});
const ledger = {owner: 'notification-browser-tests', cwd: process.cwd(), fixturePid: process.pid, fixturePort: backendPort, webPid: child.pid, webPort, command: [process.execPath, entry]};
writeFileSync('../test-results/notifications-browser/processes.json', JSON.stringify(ledger, null, 2));
console.log(JSON.stringify(ledger));
let shuttingDown = false;
function stop() {
  if (shuttingDown) return;
  shuttingDown = true;
  for (const ws of sockets.clients) ws.terminate();
  for (const stream of streams) stream.destroy();
  sockets.close(); backend.close();
  child.kill('SIGTERM');
  const timer = setTimeout(() => child.kill('SIGKILL'), 5000); timer.unref();
  child.once('exit', () => clearTimeout(timer));
}
for (const signal of ['SIGTERM', 'SIGINT']) process.on(signal, stop);
child.on('exit', code => { if (!shuttingDown) { process.exitCode = code || 1; stop(); } });
