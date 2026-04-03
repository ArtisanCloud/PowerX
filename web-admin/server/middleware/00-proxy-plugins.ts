import { defineEventHandler, getRequestURL, proxyRequest, setResponseHeader } from "h3";

export default defineEventHandler(async (event) => {
  const u = getRequestURL(event);
  const p = u.pathname || "/";
  const qs = u.search || "";

  if (!p.startsWith("/_p/")) return;

  const cfg = useRuntimeConfig(event);
  const upstream = String(cfg.upstream || process.env.POWERX_BACKEND || "http://127.0.0.1:8077").replace(/\/+$/, "");
  const target = `${upstream}${p}${qs}`;
  setResponseHeader(event, "x-px-proxy-target", target);
  setResponseHeader(event, "x-nitro-no-render", "1");
  return proxyRequest(event, target);
});

