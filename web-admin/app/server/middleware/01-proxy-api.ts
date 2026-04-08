import { defineEventHandler, getRequestURL, proxyRequest, setResponseHeader } from "h3";

export default defineEventHandler(async (event) => {
  const u = getRequestURL(event);
  const p = u.pathname || "/";
  const qs = u.search || "";

  const cfg = useRuntimeConfig(event);
  const upstream = String(
    cfg.upstream || process.env.POWERX_BACKEND || "http://127.0.0.1:8077"
  ).replace(/\/+$/, "");
  const apiBase = String(cfg.public?.apiBase || "/api/v1").replace(/\/+$/, "");

  const shouldProxy =
    p === apiBase ||
    p.startsWith(`${apiBase}/`) ||
    p === "/api/v1" ||
    p.startsWith("/api/v1/");
  if (!shouldProxy) return;

  const target = `${upstream}${p}${qs}`;
  if (String(process.env.POWERX_PROXY_DEBUG || "").toLowerCase() === "true") {
    console.info(`[proxy-api] ${event.method || "GET"} ${p} -> ${target}`);
  }
  setResponseHeader(event, "x-px-proxy-target", target);
  setResponseHeader(event, "x-px-proxy-hit", "1");
  setResponseHeader(event, "x-nitro-no-render", "1");
  return proxyRequest(event, target);
});
