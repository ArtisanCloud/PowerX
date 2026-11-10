// 对构建后的真实 Nitro 输出执行根路径/子路径检查；所有监听仅存在于本次测试。
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { spawn } from 'node:child_process';
import { resolve } from 'node:path';
import { readFile } from 'node:fs/promises';
import { setTimeout as delay } from 'node:timers/promises';
import { createHash } from 'node:crypto';

const output = resolve(process.argv[2] || '.output');
const id = '12345678-1234-1234-1234-123456789abc';
const proof = { schema: 'powerx.instance-proof/v1', challenge_id: id, nonce: 'A'.repeat(43), expires_at: '2026-01-01T00:10:00Z' };
const requests = [];
const upstream = createServer((req, res) => {
  requests.push({ path: req.url, auth: req.headers.authorization, cookie: req.headers.cookie });
  res.setHeader('content-type', 'application/json');
  res.end(JSON.stringify({ code: 200, data: proof }));
});
upstream.listen(0, '127.0.0.1'); await once(upstream, 'listening');
console.log(JSON.stringify({ owner: 'fingerprint-output-check', pid: process.pid, cwd: process.cwd(), command: process.argv, upstream_port: upstream.address().port }));
try {
  for (const base of ['/', '/console/']) {
    const reservation = createServer(); reservation.listen(0, '127.0.0.1'); await once(reservation, 'listening');
    const port = reservation.address().port; await new Promise(resolve => reservation.close(resolve));
    const child = spawn(process.execPath, [resolve(output, 'server/index.mjs')], { cwd: process.cwd(), env: { ...process.env, NODE_ENV: 'production', NITRO_HOST: '127.0.0.1', NITRO_PORT: String(port), NUXT_APP_BASE_URL: base, POWERX_BACKEND: `http://127.0.0.1:${upstream.address().port}/custom/v2` }, stdio: ['ignore', 'pipe', 'pipe'] });
    console.log(JSON.stringify({ owner: 'fingerprint-output-check', pid: child.pid, port, base, command: [process.execPath, resolve(output, 'server/index.mjs')], cwd: process.cwd() }));
    let logs = ''; child.stdout.on('data', data => { logs = (logs + data).slice(-4096); }); child.stderr.on('data', data => { logs = (logs + data).slice(-4096); });
    const exited = once(child, 'exit');
    try {
      const root = `http://127.0.0.1:${port}${base}`;
      let ready = false;
      for (let i = 0; i < 60; i++) {
        try { const response = await fetch(root, { signal: AbortSignal.timeout(1000) }); if (response.ok) { ready = true; break; } } catch { /* 限时启动检查 */ }
        await delay(100);
      }
      assert.ok(ready, logs);
      const html = await (await fetch(root)).text(); assert.match(html, /name="generator" content="PowerX"/);
      const manifestResponse = await fetch(`${root}.well-known/powerx.json`);
      assert.equal(manifestResponse.status, 200); assert.match(manifestResponse.headers.get('cache-control') || '', /no-cache/);
      const manifest = await manifestResponse.json(); assert.equal(manifest.component, 'web-admin');
      for (const artifact of manifest.artifacts) {
        const bytes = Buffer.from(await (await fetch(root + artifact.path)).arrayBuffer());
        assert.equal(bytes.length, artifact.size); assert.equal(createHash('sha256').update(bytes).digest('hex'), artifact.sha256);
      }
      const response = await fetch(`${root}.well-known/powerx-instance.json?challenge_id=${id}`, { headers: { authorization: 'Bearer visitor-must-not-forward', cookie: 'visitor=must-not-forward' } });
      assert.equal(response.status, 200, await response.clone().text()); assert.equal(response.headers.get('cache-control'), 'no-store'); assert.deepEqual(await response.json(), proof);
      assert.deepEqual(requests.at(-1), { path: `/custom/v2/system/marketplace-proof?challenge_id=${id}`, auth: undefined, cookie: undefined });
      const before = requests.length;
      const invalid = await fetch(`${root}.well-known/powerx-instance.json?challenge_id=${id}&target=https://evil.example`);
      assert.equal(invalid.status, 400); assert.equal(invalid.headers.get('cache-control'), 'no-store'); assert.equal(requests.length, before);
      console.log(`PASS ${base}: metadata, final assets, manifest cache, proof envelope, credentials and query filtering`);
    } finally {
      child.kill('SIGTERM'); await exited;
      console.log(`CLEANED Nitro PID=${child.pid} port=${port}`);
    }
  }
  const manifest = JSON.parse(await readFile(resolve(output, 'public/.well-known/powerx.json'), 'utf8'));
  console.log(`PASS release=${manifest.release_id} build_id=${manifest.build_id}`);
} finally {
  upstream.closeAllConnections(); await new Promise(resolve => upstream.close(resolve));
  console.log(`CLEANED upstream PID=${process.pid}`);
}
