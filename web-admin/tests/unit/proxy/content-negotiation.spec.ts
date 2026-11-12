// @vitest-environment node
import { createServer, type Server } from "node:http";
import { createApp, toNodeListener } from "h3";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import pluginProxy from "../../../server/middleware/00-proxy-plugins";
import apiProxy from "../../../server/middleware/01-proxy-api";

const token = "Bearer fixture-proxy-token";
const nextToken = "Bearer fixture-next-identity";
const pluginPath = "/_p/com.powerx.plugins.ai-craft/api/v1/admin/operations/assets/00000000-0000-4000-8000-000000000001/meta";
const requests: Array<{ path: string; accept?: string; authorization?: string; tenant?: string; method?: string; body: string }> = [];
type Stream = { finish: () => void; closed: Promise<void> };
const streams = new Map<string, Stream>();
const arrivals = new Map<string, (stream: Stream) => void>();
let upstream: Server;
let gateway: Server;
let origin: string;

async function listen(server: Server) {
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("FIXTURE_LISTENER_UNAVAILABLE");
  return address.port;
}

async function close(server?: Server) {
  if (!server?.listening) return;
  await new Promise<void>((resolve, reject) => {
    server.close(error => error ? reject(error) : resolve());
    server.closeAllConnections();
  });
  expect(server.listening).toBe(false);
}

async function expectUpstreamClosed(stream: { closed: Promise<void> }) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    await Promise.race([
      stream.closed,
      new Promise((_, reject) => { timer = setTimeout(() => reject(new Error("UPSTREAM_NOT_CANCELLED")), 1000); }),
    ]);
  } finally { clearTimeout(timer); }
}

beforeAll(async () => {
  vi.stubEnv("POWERX_BACKEND", "");
  upstream = createServer(async (req, res) => {
    const chunks: Buffer[] = [];
    for await (const chunk of req) chunks.push(Buffer.from(chunk));
    const request = {
      path: req.url || "/", accept: req.headers.accept,
      authorization: req.headers.authorization, tenant: req.headers.tenant_uuid as string | undefined,
      method: req.method, body: Buffer.concat(chunks).toString("base64"),
    };
    requests.push(request);
    if (![token, nextToken].includes(request.authorization || "")) {
      res.writeHead(401, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ code: "FIXTURE_UNAUTHORIZED" }));
      return;
    }
    if (request.accept === "text/event-stream") {
      const query = new URL(request.path, "http://fixture.invalid").searchParams;
      const hold = query.get("hold");
      if (hold) {
        const closed = new Promise<void>(resolve => res.once("close", resolve));
        streams.set(hold, { closed, finish: () => {
          if (!res.destroyed && !res.writableEnded) res.end('event: state\ndata: {"preview_state":"ready"}\n\n');
        } });
        arrivals.get(hold)?.(streams.get(hold)!);
        if (query.has("defer_headers")) return;
        res.writeHead(200, { "Content-Type": "text/event-stream", "Cache-Control": "no-store", "X-Accel-Buffering": "no" });
        res.write('event: state\ndata: {"preview_state":"processing"}\n\n');
      } else {
        res.writeHead(200, { "Content-Type": "text/event-stream", "Cache-Control": "no-store", "X-Accel-Buffering": "no" });
        res.end('event: state\ndata: {"preview_state":"ready"}\n\n');
      }
      return;
    }
    res.writeHead(req.method === "POST" ? 201 : 200, { "Content-Type": "application/json; charset=utf-8" });
    res.end(JSON.stringify(request));
  });
  const upstreamPort = await listen(upstream);
  vi.stubGlobal("useRuntimeConfig", () => ({ upstream: `http://127.0.0.1:${upstreamPort}`, public: { apiBase: "/api/v1" } }));
  const app = createApp().use(pluginProxy).use(apiProxy);
  gateway = createServer(toNodeListener(app));
  const gatewayPort = await listen(gateway);
  origin = `http://127.0.0.1:${gatewayPort}`;
  console.info(JSON.stringify({ owned_proxy_fixture: { pid: process.pid, command: "vitest content-negotiation.spec.ts", cwd: process.cwd(), ports: [upstreamPort, gatewayPort] } }));
});

afterAll(async () => {
  for (const stream of streams.values()) stream.finish();
  try { await Promise.all([close(gateway), close(upstream)]); }
  finally { vi.unstubAllGlobals(); vi.unstubAllEnvs(); }
});

function request(path: string, accept = "application/json", extra: RequestInit = {}) {
  return fetch(origin + path, {
    ...extra, signal: AbortSignal.timeout(3000),
    headers: { Accept: accept, Authorization: token, tenant_uuid: "fixture-tenant", ...extra.headers },
  });
}

describe("Core proxy content negotiation through real h3 middleware", () => {
  it("forwards plugin preview events as SSE instead of a successful JSON response", async () => {
    const response = await request(pluginPath, "text/event-stream");
    const body = await response.text();
    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toBe("text/event-stream");
    expect(body).toContain('event: state\ndata: {"preview_state":"ready"}');
    expect(requests.at(-1)).toMatchObject({ accept: "text/event-stream", authorization: token, tenant: "fixture-tenant" });
  });

  it.each([pluginPath, `/__up${pluginPath}`, "/api/v1/admin/media/assets/fixture/derivatives/events"])("preserves negotiation and identity headers on %s", async path => {
    for (const accept of ["text/event-stream", "application/json", "image/*", 'application/vnd.powerx.asset+json; profile="CaseSensitive", application/json;q=0.8']) {
      const response = await request(`${path}?tenant_uuid=fixture-tenant&__px_iframe=1`, accept);
      const body = await response.text();
      expect(response.status).toBe(200);
      expect(requests.at(-1)).toMatchObject({ accept, authorization: token, tenant: "fixture-tenant" });
      expect(response.headers.get("content-type")).toContain(accept === "text/event-stream" ? "text/event-stream" : "application/json");
      if (accept !== "text/event-stream") expect(JSON.parse(body).path).toContain("tenant_uuid=fixture-tenant");
      if (path !== "/api/v1/admin/media/assets/fixture/derivatives/events") expect(requests.at(-1)?.path).not.toContain("__px_iframe");
    }
  });

  it("delivers the initial event before upstream completion without another state request", async () => {
    const count = requests.length;
    const response = await request(`${pluginPath}?hold=preview`, "text/event-stream");
    expect(response.headers.get("content-type")).toBe("text/event-stream");
    const reader = response.body!.getReader();
    try {
      const first = await reader.read();
      expect(new TextDecoder().decode(first.value)).toContain('"preview_state":"processing"');
      expect(requests.length).toBe(count + 1);
      streams.get("preview")!.finish();
      let tail = "";
      for (;;) { const chunk = await reader.read(); if (chunk.done) break; tail += new TextDecoder().decode(chunk.value); }
      expect(tail).toContain('"preview_state":"ready"');
      expect(requests.length).toBe(count + 1);
    } finally { await reader.cancel(); reader.releaseLock(); }
  });

  it.each([
    [pluginPath, "GET"],
    [`/__up${pluginPath}`, "GET"],
    ["/api/v1/admin/media/assets/fixture/derivatives/events", "GET"],
    ["/api/v1/admin/ops/backup/cleanup-reviews/fixture/retentions", "POST"],
    ["/api/v1/admin/ops/backup/events", "GET"],
    ["/api/v1/admin/plugins/installations/fixture/events", "GET"],
  ])("keeps stream negotiation and upstream cancellation on %s (%s)", async (path, method) => {
    const controller = new AbortController();
    const response = await fetch(`${origin}${path}?hold=${encodeURIComponent(path)}`, { method, headers: { Accept: "text/event-stream", Authorization: token }, signal: controller.signal });
    expect(response.headers.get("content-type")).toBe("text/event-stream");
    const reader = response.body!.getReader();
    try {
      expect(new TextDecoder().decode((await reader.read()).value)).toContain("processing");
      controller.abort();
      await expectUpstreamClosed(streams.get(path)!);
    } finally { controller.abort(); await reader.cancel().catch(() => {}); reader.releaseLock(); }
  });

  it("cancels an upstream request even before it sends response headers", async () => {
    const key = "before-headers";
    const arrived = new Promise<Stream>(resolve => arrivals.set(key, resolve));
    const controller = new AbortController();
    const pending = fetch(`${origin}${pluginPath}?hold=${key}&defer_headers=1`, {
      headers: { Accept: "text/event-stream", Authorization: token }, signal: controller.signal,
    }).catch(error => error);
    try {
      const stream = await arrived;
      controller.abort();
      expect((await pending).name).toBe("AbortError");
      await expectUpstreamClosed(stream);
    } finally { controller.abort(); arrivals.delete(key); }
  });

  it("releases an old identity subscription before another identity opens its stream", async () => {
    const oldIdentity = new AbortController(), nextIdentity = new AbortController();
    const readers: ReadableStreamDefaultReader<Uint8Array>[] = [];
    try {
      const old = await fetch(`${origin}${pluginPath}?hold=identity-old`, {
        headers: { Accept: "text/event-stream", Authorization: token }, signal: oldIdentity.signal,
      });
      const oldReader = old.body!.getReader(); readers.push(oldReader);
      await oldReader.read();
      oldIdentity.abort();
      await expectUpstreamClosed(streams.get("identity-old")!);

      const next = await fetch(`${origin}${pluginPath}?hold=identity-next`, {
        headers: { Accept: "text/event-stream", Authorization: nextToken }, signal: nextIdentity.signal,
      });
      const nextReader = next.body!.getReader(); readers.push(nextReader);
      expect(new TextDecoder().decode((await nextReader.read()).value)).toContain("processing");
      expect(requests.at(-1)?.authorization).toBe(nextToken);
      nextIdentity.abort();
      await expectUpstreamClosed(streams.get("identity-next")!);
    } finally {
      oldIdentity.abort(); nextIdentity.abort();
      for (const reader of readers) { await reader.cancel().catch(() => {}); reader.releaseLock(); }
    }
  });

  it("keeps the host cookie fallback for plugin requests without replacing explicit credentials", async () => {
    const response = await fetch(origin + pluginPath, { headers: { Accept: "text/event-stream", Cookie: "access_token=fixture-proxy-token" }, signal: AbortSignal.timeout(3000) });
    await response.text();
    expect(response.headers.get("content-type")).toBe("text/event-stream");
    expect(response.headers.get("x-px-auth-injected")).toBe("1");
    expect(requests.at(-1)?.authorization).toBe(token);
  });

  it("preserves upstream authorization failures", async () => {
    const response = await request(pluginPath, "text/event-stream", { headers: { Authorization: "Bearer fixture-invalid-token" } });
    expect(response.status).toBe(401);
    expect(await response.json()).toMatchObject({ code: "FIXTURE_UNAUTHORIZED" });
    expect(requests.at(-1)?.authorization).toBe("Bearer fixture-invalid-token");
  });

  it("keeps upload bytes, query parameters and upstream status intact", async () => {
    const body = new Uint8Array([0, 255, 1, 2, 128, 10]);
    const response = await request(pluginPath + "?part=1", "application/json", { method: "POST", body, headers: { "Content-Type": "application/octet-stream" } });
    expect(response.status).toBe(201);
    expect(await response.json()).toMatchObject({ method: "POST", body: Buffer.from(body).toString("base64"), path: pluginPath + "?part=1", accept: "application/json" });
  });
});
