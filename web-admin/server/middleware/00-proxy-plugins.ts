import {
  defineEventHandler,
  getHeader,
  getRequestURL,
  proxyRequest,
  setResponseHeader,
} from "h3";

export default defineEventHandler(async (event) => {
  const u = getRequestURL(event);
  const p = u.pathname || "/";
  const qs = u.search || "";

  // /_p/** 路由复用：
  // - 顶层页面刷新（document）走 Nuxt 页面壳（保留侧栏）
  // - iframe 文档与其资源请求走后端插件反代
  if (!p.startsWith("/_p/")) return;

  const cfg = useRuntimeConfig(event);
  const upstream = String(cfg.upstream || process.env.POWERX_BACKEND || "http://127.0.0.1:8077").replace(/\/+$/, "");
  const accept = String(getHeader(event, "accept") || "").toLowerCase();
  const secFetchDest = String(getHeader(event, "sec-fetch-dest") || "").toLowerCase();
  const isHtml = accept.includes("text/html");
  const isIframeDoc = secFetchDest === "iframe";

  // 顶层刷新 /_p/**：交给 Nuxt 页面，不做代理
  if (isHtml && !isIframeDoc) return;

  const target = `${upstream}${p}${qs}`;
  setResponseHeader(event, "x-px-proxy-target", target);
  setResponseHeader(event, "x-nitro-no-render", "1");
  return proxyRequest(event, target);
});
