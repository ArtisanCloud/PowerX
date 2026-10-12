import {
  defineEventHandler,
  getCookie,
  getHeader,
  getRequestURL,
  proxyRequest,
  setResponseHeader,
  setResponseStatus,
} from "h3";

const HOP_BY_HOP_HEADERS = new Set([
  "connection",
  "upgrade",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "host",
  "content-length",
]);

function buildProxyHeaders(input: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(input || {})) {
    const key = String(k || "").toLowerCase();
    if (!key || HOP_BY_HOP_HEADERS.has(key)) continue;
    if (typeof v === "string") out[k] = v;
  }
  return out;
}

export default defineEventHandler(async (event) => {
  const u = getRequestURL(event);
  const p = u.pathname || "/";
  const forwardSearch = new URLSearchParams(u.searchParams);
  forwardSearch.delete("__px_iframe");
  const qs = forwardSearch.toString() ? `?${forwardSearch.toString()}` : "";
  const iframeForced = ["1", "true", "yes"].includes(
    String(u.searchParams.get("__px_iframe") || "").toLowerCase()
  );

  // 路由复用：
  // - 顶层页面刷新（document）走 Nuxt 页面壳（保留侧栏）
  // - iframe 文档与其资源请求走后端插件反代
  // - /__up/** 为 iframe 专用直通反代，避免与 Nuxt 页面路由 /_p/** 冲突
  const isUpstreamBypass = p.startsWith("/__up/");
  const isPluginRoute = p.startsWith("/_p/");
  if (!isUpstreamBypass && !isPluginRoute) return;

  const cfg = useRuntimeConfig(event);
  const upstream = String(process.env.POWERX_BACKEND || cfg.upstream || "http://127.0.0.1:8077").replace(/\/+$/, "");
  const accept = String(getHeader(event, "accept") || "").toLowerCase();
  if (isPluginRoute) {
    const secFetchDest = String(getHeader(event, "sec-fetch-dest") || "").toLowerCase();
    const isHtml = accept.includes("text/html");
    const isIframeDoc = secFetchDest === "iframe";
    const referer = String(getHeader(event, "referer") || "");
    const inIframeFlow =
      iframeForced ||
      isIframeDoc ||
      referer.includes("__px_iframe=1") ||
      referer.includes("/__up/");

    // 顶层刷新 /_p/**：交给 Nuxt 页面，不做代理
    if (isHtml && !inIframeFlow) return;
  }

  // 代理层补 Bearer：插件 iframe 文档与其 API 常常不会主动携带 Authorization，
  // 这里从宿主登录 cookie 回填，避免插件误判未登录并跳到登录页。
  const hasAuthHeader = Boolean(String(getHeader(event, "authorization") || "").trim());
  if (!hasAuthHeader) {
    const bearer = String(
      getCookie(event, "access_token") ||
      ""
    ).trim();
    if (bearer) {
      event.node.req.headers.authorization = `Bearer ${bearer}`;
      setResponseHeader(event, "x-px-auth-injected", "1");
    } else {
      setResponseHeader(event, "x-px-auth-injected", "0");
    }
  }

  let targetPath = isUpstreamBypass ? p.replace(/^\/__up/, "") || "/" : p;

  // 兼容插件内以 /_p/<plugin>/api/v1/admin/user/auth/* 访问宿主认证接口：
  // 统一重写到宿主真实入口 /api/v1/admin/user/auth/*
  const scopedUserAuth = targetPath.match(/^\/_p\/[^/]+\/api\/v1\/admin\/user\/auth\/(.+)$/);
  if (scopedUserAuth?.[1]) {
    targetPath = `/api/v1/admin/user/auth/${scopedUserAuth[1]}`;
  }

  // 兼容旧插件历史探针：/api/v1/admin/iam/auth/mode
  // 主系统当前无该路由；返回“宿主委托模式”响应，避免插件误判为本地登录模式。
  if (/^\/_p\/[^/]+\/api\/v1\/admin\/iam\/auth\/mode$/.test(targetPath)) {
    setResponseStatus(event, 200);
    return {
      code: 200,
      message: "success",
      data: {
        mode: "delegated",
        auth_mode: "delegated",
        password_enabled: false,
        passwordEnabled: false,
        federated_enabled: false,
        federatedEnabled: false,
        host_managed: true,
        hostManaged: true,
      },
      timestamp: Math.floor(Date.now() / 1000),
    };
  }

  // Embedded clients consume the host session; they cannot rotate or clear it independently.
  if (/^\/api\/v1\/admin\/user\/auth\/(login|refresh|logout)$/.test(targetPath)) {
    setResponseStatus(event, 409);
    return { code: 409, error_code: 'AUTH_SESSION_HOST_MANAGED', message: 'AUTH_SESSION_HOST_MANAGED' };
  }

  const target = `${upstream}${targetPath}${qs}`;
  const referer = String(getHeader(event, "referer") || "");
  const isIframeFlow = iframeForced || referer.includes("__px_iframe=1") || referer.includes("/__up/");
  const shouldInjectBridge =
    (isUpstreamBypass || (isPluginRoute && isIframeFlow)) &&
    accept.includes("text/html") &&
    /^\/_p\/[^/]+\/admin(?:\/|$)/.test(targetPath);

  if (shouldInjectBridge) {
    const reqHeaders = buildProxyHeaders(event.node.req.headers as Record<string, unknown>);

    const resp = await fetch(target, {
      method: String(event.method || "GET").toUpperCase(),
      headers: reqHeaders,
      redirect: "manual",
    });

    setResponseStatus(event, resp.status);
    resp.headers.forEach((value, key) => {
      const lower = key.toLowerCase();
      if (lower === "content-length" || lower === "content-encoding" || lower === "transfer-encoding") return;
      setResponseHeader(event, key, value);
    });
    setResponseHeader(event, "x-px-proxy-target", target);
    setResponseHeader(event, "x-nitro-no-render", "1");

    const contentType = String(resp.headers.get("content-type") || "").toLowerCase();
    if (!contentType.includes("text/html")) {
      return await resp.text();
    }

    let html = await resp.text();
    if (!html.includes("/powerx-bridge-client.js")) {
      const marker = "</head>";
      const script = '<script src="/powerx-bridge-client.js?v=2"></script>';
      if (html.includes(marker)) {
        html = html.replace(marker, `${script}${marker}`);
      } else {
        html = `${script}${html}`;
      }
    }
    setResponseHeader(event, "content-type", "text/html; charset=utf-8");
    return html;
  }

  setResponseHeader(event, "x-px-proxy-target", target);
  setResponseHeader(event, "x-nitro-no-render", "1");
  return proxyRequest(event, target);
});
