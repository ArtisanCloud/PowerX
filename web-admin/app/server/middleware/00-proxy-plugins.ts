import { defineEventHandler, getRequestURL, proxyRequest, setResponseHeader } from "h3";

export default defineEventHandler(async (event) => {
  const u = getRequestURL(event);
  const p = u.pathname || "/";
  const qs = u.search || "";

  const cfg = useRuntimeConfig(event);
  const upstream = String(
    cfg.upstream || process.env.POWERX_BACKEND || "http://127.0.0.1:8077"
  ).replace(/\/+$/, "");

  // 只要是 /_p/** ：“只代理，不渲染”到后端，避免画中画
  if (p.startsWith("/_p/")) {
    const target = `${upstream}${p}${qs}`;
    setResponseHeader(event, "x-px-proxy-target", target); // Network 可见真实后端
    setResponseHeader(event, "x-nitro-no-render", "1"); // 禁止 Nuxt 包壳
    return proxyRequest(event, target);
  }
});
