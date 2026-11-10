import { chromium } from "../../web-admin/node_modules/playwright/index.mjs";
import { readFileSync, writeFileSync, statSync, unlinkSync } from "node:fs";
import { resolve } from "node:path";
import http from "node:http";
import net from "node:net";
import { execFileSync } from "node:child_process";
const dir = resolve(
    process.env.POWERX_ARCHIVE_COMMAND_UI_ARTIFACTS ||
      "tmp/storage-archive-command-drain-20261110",
  ),
  privateFile = resolve(dir, "http-private.json"),
  owner = JSON.parse(readFileSync(privateFile)),
  token = owner.token;
if (
  owner.owner !== "codex archive disposition fixture" ||
  new URL(owner.url).hostname !== "127.0.0.1" ||
  [3030, 8077].includes(Number(new URL(owner.url).port)) ||
  (statSync(privateFile).mode & 0o077) !== 0
)
  throw new Error("Refuse non-owned or non-private archive fixture");
const beforeHeaders =
  process.env.POWERX_ARCHIVE_DISPOSITION_LOSS_MODE === "pre-headers";
const result = {
    checks: [],
    requests: [],
    errors: [],
    application_submits: [],
  },
  upstreams = new Set();
let browser,
  context,
  page,
  hideCatalog = false,
  dropEvents = false;
let loseReply = false,
  lookupMode = "native",
  capturedReceipt;
const eventResponses = new Set();
const processOwner = {
  owner: "codex archive fault browser proxy",
  pid: process.pid,
  command: "node scripts/ops/test-storage-archive-command-drain-ui.mjs",
  cwd: process.cwd(),
  ports: [],
};
const base = "/api/v1/admin/ops/backup";
const proxy = http.createServer((req, res) => {
  const u = new URL(req.url, "http://127.0.0.1:3030"),
    controlled =
      u.pathname.startsWith(base) ||
      u.pathname.startsWith("/api/v1/admin/monitor/backup");
  if (controlled)
    result.requests.push({
      path: u.pathname,
      method: req.method,
      at: Date.now(),
    });
  if (controlled && req.method === "POST" && u.pathname.endsWith("/closures")) {
    const entry = result.requests.at(-1);
    let body = "";
    req.on("data", (b) => {
      if (body.length < 4096) body += b.toString();
    });
    req.on("end", () => {
      try {
        entry.request_id = JSON.parse(body).request_id;
      } catch {
        entry.request_id = "unparsed";
      }
    });
  }
  if (
    controlled &&
    req.method === "GET" &&
    u.pathname.includes("/interruption-requests/") &&
    lookupMode !== "native"
  ) {
    res.writeHead(
      lookupMode === "missing" ? 404 : lookupMode === "unavailable" ? 503 : 200,
      { "content-type": "application/json" },
    );
    res.end(
      JSON.stringify(
        lookupMode === "mismatch"
          ? {
              code: 200,
              data: {
                ...capturedReceipt,
                job_id: "11111111-1111-4111-8111-111111111111",
              },
            }
          : { message: "owned lookup unavailable" },
      ),
    );
    return;
  }
  if (
    controlled &&
    hideCatalog &&
    req.method === "GET" &&
    u.pathname.includes("/archives/catalog")
  ) {
    res.writeHead(503, { "content-type": "application/json" });
    res.end(
      JSON.stringify({
        code: 503,
        message: "owned archive metadata unavailable",
      }),
    );
    return;
  }
  if (controlled && dropEvents && u.pathname.endsWith("/events")) {
    res.writeHead(503);
    res.end();
    return;
  }
  if (
    !controlled &&
    !["GET", "HEAD", "OPTIONS"].includes(req.method) &&
    !u.pathname.includes("/auth/")
  ) {
    res.writeHead(403);
    res.end("unrelated mutation blocked");
    return;
  }
  const remote = controlled ? new URL(u.pathname + u.search, owner.url) : u;
  if (
    !controlled &&
    !["127.0.0.1:3030", "localhost:3030"].includes(remote.host)
  ) {
    res.writeHead(403);
    res.end();
    return;
  }
  const headers = controlled
    ? {
        authorization: "Bearer " + token,
        accept: req.headers.accept || "application/json",
        "content-type": "application/json",
      }
    : { ...req.headers, host: remote.host };
  delete headers["proxy-connection"];
  const up = http.request(remote, { method: req.method, headers }, (r) => {
    if (controlled && u.pathname.endsWith("/events")) {
      eventResponses.add(res);
      res.on("close", () => eventResponses.delete(res));
    }
    if (
      controlled &&
      req.method === "POST" &&
      u.pathname.endsWith("/closures") &&
      loseReply
    ) {
      loseReply = false;
      const chunks = [];
      r.on("data", (b) => chunks.push(b));
      r.on("end", () => {
        const payload = JSON.parse(Buffer.concat(chunks).toString());
        capturedReceipt = payload.data;
        if (beforeHeaders) {
          res.destroy();
          return;
        }
        res.writeHead(200, { "content-type": "application/json" });
        res.flushHeaders();
        res.write('{"code":200,"data":');
        setTimeout(() => res.destroy(), 80);
      });
      return;
    }
    res.writeHead(r.statusCode, r.headers);
    r.pipe(res);
  });
  upstreams.add(up);
  up.on("close", () => upstreams.delete(up));
  up.on("error", () => {
    if (!res.headersSent) res.writeHead(502);
    res.end();
  });
  res.on("close", () => up.destroy());
  req.pipe(up);
});
const relaySockets = new Set();
const allowedRelay = (host, port) =>
  ["127.0.0.1", "localhost", "::1"].includes(host) &&
  [3030, 8077, 5173].includes(Number(port));
proxy.on("connect", (req, socket, head) => {
  const u = new URL("http://" + req.url);
  if (!allowedRelay(u.hostname, u.port)) {
    socket.end("HTTP/1.1 403 Forbidden\r\n\r\n");
    return;
  }
  const remote = net.connect(Number(u.port), u.hostname, () => {
    socket.write("HTTP/1.1 200 Connection Established\r\n\r\n");
    if (head.length) remote.write(head);
    socket.pipe(remote);
    remote.pipe(socket);
  });
  relaySockets.add(remote);
  relaySockets.add(socket);
  (result.ws_tunnels ??= []).push({ host: u.hostname, port: Number(u.port) });
  for (const current of [remote, socket]) {
    current.on("error", () => {
      remote.destroy();
      socket.destroy();
    });
    current.on("close", () => {
      relaySockets.delete(current);
      remote.destroy();
      socket.destroy();
    });
  }
});
proxy.on("upgrade", (req, socket, head) => {
  const u = new URL(req.url, "http://127.0.0.1:3030");
  const port = Number(u.port || 80);
  if (!allowedRelay(u.hostname, port)) {
    socket.destroy();
    return;
  }
  const remote = net.connect(port, u.hostname, () => {
    const headers = { ...req.headers, host: u.host };
    delete headers["proxy-connection"];
    remote.write(
      `${req.method} ${u.pathname + u.search} HTTP/${req.httpVersion}\r\n` +
        Object.entries(headers)
          .map(([k, v]) => `${k}: ${v}\r\n`)
          .join("") +
        "\r\n",
    );
    if (head.length) remote.write(head);
    socket.pipe(remote);
    remote.pipe(socket);
  });
  relaySockets.add(remote);
  relaySockets.add(socket);
  (result.ws_upgrades ??= []).push({
    host: u.hostname,
    port,
    path: u.pathname,
  });
  for (const current of [remote, socket]) {
    current.on("error", () => {
      remote.destroy();
      socket.destroy();
    });
    current.on("close", () => {
      relaySockets.delete(current);
      remote.destroy();
      socket.destroy();
    });
  }
});
const assert = (v, m) => {
    if (!v) throw new Error(m);
  },
  mark = (m) => {
    result.checks.push(m);
    console.log(m);
  };
const ownedPost = async (action) => {
  const r = await fetch(owner.url + base + "/__owned-fixture/" + action, {
    method: "POST",
    headers: { authorization: "Bearer " + token },
  });
  assert(r.ok, "owned transition failed: " + action);
  return r.json();
};
try {
  await new Promise((done) => proxy.listen(0, "127.0.0.1", done));
  processOwner.ports.push(proxy.address().port);
  browser = await chromium.launch({
    headless: true,
    proxy: {
      server: "http://127.0.0.1:" + proxy.address().port,
      bypass: "<-loopback>",
    },
  });
  const rows = execFileSync("ps", ["-axo", "pid=,ppid=,comm="], {
      encoding: "utf8",
    })
      .trim()
      .split("\n")
      .map((l) => {
        const m = l.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/);
        return m ? { pid: +m[1], ppid: +m[2], binary: m[3] } : null;
      })
      .filter(Boolean),
    ids = new Set([process.pid]);
  for (let i = 0; i < 8; i++)
    for (const r of rows) if (ids.has(r.ppid)) ids.add(r.pid);
  processOwner.children = rows.filter(
    (r) => ids.has(r.pid) && r.pid !== process.pid,
  );
  writeFileSync(
    resolve(dir, "ui-owner.json"),
    JSON.stringify(processOwner, null, 2),
  );
  context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    locale: "zh-CN",
  });
  await context.exposeBinding("__ownedDispositionSubmit", (_source, key) =>
    result.application_submits.push(key),
  );
  await context.addInitScript(() => {
    localStorage.setItem("welcome_guide_completed", "true");
    const send = window.fetch.bind(window);
    window.fetch = (input, options) => {
      const url =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? input.href
            : input.url;
      if (url.endsWith("/closures")) {
        let id;
        try {
          id = JSON.parse(options?.body || "{}").request_id;
        } catch {}
        void window.__ownedDispositionSubmit(id || "unparsed");
      }
      return send(input, options);
    };
  });
  page = await context.newPage();
  page.on("pageerror", (e) => result.errors.push(e.message));
  await page.goto("http://127.0.0.1:3030/users/login", {
    waitUntil: "domcontentloaded",
    timeout: 60000,
  });
  await page
    .locator("input[name=identifier]")
    .fill(process.env.POWERX_CHECK_USER);
  await page
    .locator("input[name=password]")
    .fill(process.env.POWERX_CHECK_PASSWORD);
  await page.locator("form button[type=submit]").click();
  await page.waitForURL((u) => !u.pathname.includes("/users/login"), {
    timeout: 45000,
    waitUntil: "domcontentloaded",
  });
  const originalJob = readFileSync(
    resolve(owner.directory, "jobs", owner.job_id + ".json"),
    "utf8",
  );
  await page.goto("http://127.0.0.1:3030/monitor/storage", {
    waitUntil: "domcontentloaded",
    timeout: 60000,
  });
  const zh = JSON.parse(readFileSync("web-admin/i18n/locales/zh.json"));
  const workspace = page.getByTestId("storage-workspace"),
    nav = workspace.getByTestId("backup-section-tabs").first();
  await nav
    .getByRole("tab", { name: zh.storageGov.archives, exact: true })
    .click();
  const panel = page.getByTestId("archive-governance");
  await panel.getByTestId("archive-command-unconfirmed").waitFor();
  await panel
    .getByTestId("archive-command-unconfirmed")
    .getByRole("button", { name: owner.job_id, exact: true })
    .click();
  const dialog = page.getByTestId("archive-disposition-dialog");
  await dialog.getByTestId("archive-command-protection").waitFor();
  const tabs = dialog.getByTestId("backup-section-tabs").first();
  const choose = async (value, width) => {
    if (width >= 640)
      await tabs
        .getByRole("tab", {
          name: zh.storageGov.archive.disposition.tabs[value],
          exact: true,
        })
        .click();
    else await tabs.locator("select[data-section-select]").selectOption(value);
  };
  for (const theme of ["light", "dark"]) {
    await page.evaluate(
      (theme) => (window.useNuxtApp().$colorMode.preference = theme),
      theme,
    );
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 1000 });
      await choose("overview", width);
      await choose("verification", width);
      assert(
        await dialog.getByTestId("archive-disposition-verify").isDisabled(),
        "unconfirmed process enables capture review",
      );
      assert(
        !(await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth + 1,
        )),
        "page overflow",
      );
      await page.screenshot({
        path: resolve(dir, `${theme}-${width}-command-protection.png`),
        fullPage: true,
        animations: "disabled",
      });
      mark(
        `${theme} ${width} command protection visible; verification disabled across task tabs`,
      );
    }
  }
  dropEvents = true;
  for (const response of eventResponses) response.destroy();
  await dialog
    .getByTestId("archive-disposition-disconnected")
    .waitFor({ timeout: 15000 });
  assert(
    (await dialog.getByTestId("archive-command-protection").count()) === 1,
    "disconnect erased protection",
  );
  dropEvents = false;
  await dialog
    .getByTestId("archive-disposition-disconnected")
    .waitFor({ state: "hidden", timeout: 20000 });
  const reads = () =>
    result.requests.filter(
      (r) => r.method === "GET" && !r.path.endsWith("/events"),
    ).length;
  await page.waitForTimeout(600);
  const before = reads();
  await page.waitForTimeout(1800);
  assert(reads() === before, "idle status polling");
  await ownedPost("drain");
  await dialog
    .getByTestId("archive-command-protection")
    .waitFor({ state: "hidden", timeout: 15000 });
  await dialog.getByTestId("archive-disposition-verify").waitFor();
  assert(
    await dialog.getByTestId("archive-disposition-verify").isEnabled(),
    "SSE did not refresh current local proof",
  );
  assert(
    readFileSync(
      resolve(owner.directory, "jobs", owner.job_id + ".json"),
      "utf8",
    ) === originalJob,
    "proof update changed original task",
  );
  assert(
    readFileSync(resolve(owner.source, "photo.jpg"), "utf8") ===
      "owned partial content",
    "source changed",
  );
  assert(
    !result.requests.some((r) => r.method !== "GET"),
    "browser submitted mutation",
  );
  assert(
    !result.requests.some((r) => r.path.endsWith("/interruption-review")),
    "SSE auto-started content verification",
  );
  mark(
    "SSE reconnect preserves protection with no idle polling; controlled proof update refreshes UI without changing task or starting verification",
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: zh.common.close, exact: true })
    .click();
  await nav
    .locator("select[data-section-select]")
    .first()
    .selectOption("overview");
  await workspace.getByTestId("storage-archive-attention").waitFor();
  mark(
    "Original interrupted task remains discoverable after local drain and tab switch",
  );
  assert(!result.errors.length, result.errors.join(";"));
  await page.goto("http://127.0.0.1:3030/users/profile", {
    waitUntil: "domcontentloaded",
  });
  await page.waitForTimeout(350);
  assert(eventResponses.size === 0, "SSE leaked");
  result.result = "passed";
  result.job_id = owner.job_id;
} catch (e) {
  result.result = "failed";
  result.error = e.message;
  if (page)
    await page
      .screenshot({ path: resolve(dir, "failure.png"), fullPage: true })
      .catch(() => {});
} finally {
  if (context) await context.close();
  if (browser) await browser.close();
  for (const up of upstreams) up.destroy();
  for (const socket of relaySockets) socket.destroy();
  proxy.closeAllConnections?.();
  await new Promise((done) => proxy.close(done));
  writeFileSync(
    resolve(dir, "ui-result.json"),
    JSON.stringify(result, null, 2),
  );
  writeFileSync(
    resolve(dir, "ui-owner.json"),
    JSON.stringify(
      { ...processOwner, browser_closed: true, proxy_closed: true },
      null,
      2,
    ),
  );
  await ownedPost("finish").catch(() => {});
  unlinkSync(privateFile);
  writeFileSync(
    resolve(dir, "http-owner.json"),
    JSON.stringify(
      Object.fromEntries(Object.entries(owner).filter(([k]) => k !== "token")),
      null,
      2,
    ),
  );
}
console.log(
  JSON.stringify({
    result: result.result,
    error: result.error,
    checks: result.checks,
    errors: result.errors,
  }),
);
if (result.result !== "passed") process.exitCode = 1;
