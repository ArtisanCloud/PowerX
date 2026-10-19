import { createServer } from "node:http";
import { spawn } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { WebSocketServer } from "ws";

// Controlled UI data only. Real role/source/ledger writes are exercised in Go/PostgreSQL.
const webPort = 3495,
  backendPort = 3496;
const tenant = "11111111-1111-4111-8111-111111111111";
const member = "22222222-2222-4222-8222-222222222222";
const context = {
  is_root: true,
  current_tenant_uuid: tenant,
  current_member_id: 1,
  current_member_uuid: member,
  user: {
    id: 1,
    email: "root-role@example.invalid",
    display_name: "Root 角色验收",
    status: 1,
    is_root: true,
  },
  members: [
    {
      tenant_uuid: tenant,
      tenant_name: "验收租户",
      member_id: 1,
      member_uuid: member,
      is_admin: true,
    },
  ],
};
const roles = [
  {
    id: 1,
    uuid: "root-fixture",
    name: "System Root",
    code: "root",
    scope: "system",
    builtin: true,
  },
  {
    id: 2,
    uuid: "admin-fixture",
    name: "Tenant Admin",
    code: "role_admin",
    scope: "tenant",
    tenant_uuid: tenant,
    builtin: true,
  },
  {
    id: 3,
    uuid: "user-fixture",
    name: "Tenant User",
    code: "role_user",
    scope: "tenant",
    tenant_uuid: tenant,
    builtin: true,
  },
].map((role) => ({ ...role, createdAt: "2026-10-19T00:00:00Z" }));
const requests = [];
const json = (res, data) => {
  res.writeHead(200, { "content-type": "application/json" });
  res.end(JSON.stringify({ code: 200, data }));
};
const sockets = new WebSocketServer({ noServer: true });
const backend = createServer((req, res) => {
  const url = new URL(req.url, `http://127.0.0.1:${backendPort}`);
  if (url.pathname.endsWith("/__root_role_fixture"))
    return json(res, { requests });
  requests.push({ method: req.method, path: url.pathname });
  if (url.pathname.endsWith("/setup/status"))
    return json(res, {
      configured: true,
      requires_login: true,
      install_status: "installed",
    });
  if (url.pathname.endsWith("/user/auth/me/context")) return json(res, context);
  if (url.pathname.endsWith("/admin/menus"))
    return json(res, {
      categories: [
        {
          id: "settings",
          title: "系统设置",
          children: [
            {
              id: "roles",
              title: "角色管理",
              path: "/settings/roles",
              visible: true,
              origin: "core",
            },
            {
              id: "users",
              title: "用户管理",
              path: "/settings/users",
              visible: true,
              origin: "core",
            },
          ],
        },
      ],
    });
  if (url.pathname.endsWith("/iam/roles"))
    return json(res, {
      items: roles,
      pagination: { total: roles.length, page: 1, page_size: 20, pages: 1 },
    });
  if (/\/roles\/\d+\/permissions$/.test(url.pathname))
    return json(res, { items: [] });
  if (url.pathname.endsWith("/permissions/plugin-catalog"))
    return json(res, { plugins: [] });
  return json(res, {
    items: [],
    pagination: { total: 0, page: 1, page_size: 20, pages: 1 },
  });
});
backend.on("upgrade", (req, socket, head) =>
  sockets.handleUpgrade(req, socket, head, (ws) => {
    ws.on("message", (raw) => {
      const cmd = JSON.parse(raw.toString());
      if (cmd.type === "subscribe")
        ws.send(JSON.stringify({ type: "ack", payload: { topic: cmd.topic } }));
    });
  }),
);
await new Promise((yes, no) => {
  backend.once("error", no);
  backend.listen(backendPort, "127.0.0.1", yes);
});
const entry = resolve(".output-root-role-validation/server/index.mjs");
const child = spawn(process.execPath, [entry], {
  cwd: process.cwd(),
  stdio: "inherit",
  env: {
    ...process.env,
    NODE_ENV: "production",
    HOST: "127.0.0.1",
    PORT: String(webPort),
    POWERX_BACKEND: `http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_API_BASE: "/api/v1",
    NUXT_PUBLIC_POWERX_CORE_BASE: `http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_UPSTREAM_ORIGIN: `http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_WS_ORIGIN: `ws://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_WS_PATH: `ws://127.0.0.1:${backendPort}/api/ws`,
    NODE_PATH: resolve("node_modules"),
  },
});
mkdirSync("../test-results/root-role-browser", { recursive: true });
writeFileSync(
  "../test-results/root-role-browser/processes.json",
  JSON.stringify(
    {
      owner: "root-role-browser-tests",
      cwd: process.cwd(),
      fixturePid: process.pid,
      fixturePort: backendPort,
      webPid: child.pid,
      webPort,
      command: [process.execPath, entry],
    },
    null,
    2,
  ),
);
let stopping = false;
const stop = () => {
  if (stopping) return;
  stopping = true;
  for (const ws of sockets.clients) ws.terminate();
  sockets.close();
  backend.close();
  child.kill("SIGTERM");
  const timer = setTimeout(() => child.kill("SIGKILL"), 5000);
  timer.unref();
  child.once("exit", () => clearTimeout(timer));
};
for (const signal of ["SIGTERM", "SIGINT"]) process.on(signal, stop);
child.on("exit", (code) => {
  if (!stopping) {
    process.exitCode = code || 1;
    stop();
  }
});
