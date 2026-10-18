import { createError, defineEventHandler, getHeader, getRequestURL, proxyRequest, setResponseHeader } from "h3";
import { proxyBackupStream } from "../utils/backup-stream-proxy";

// 同一站点、同一 Core 地址；维护模式接管原端口，无需额外状态服务。
// 旧部署的状态地址仅对严格限定的只读 jobs 路径保留兼容。
export default defineEventHandler(async (event) => {
  const url = getRequestURL(event);
  if (!url.pathname.startsWith("/__powerx_recovery/")) return;
  if (url.search) throw createError({ statusCode: 400, statusMessage: "Recovery URLs do not accept credentials or parameters" });
  const job = /^\/__powerx_recovery\/jobs\/[0-9a-f-]{36}(?:\/events)?$/.test(url.pathname);
  const resource = url.pathname.slice('/__powerx_recovery/'.length);
  const method = event.method;
  const allowed = job ? method === 'GET' : (
    (['', 'app.js', 'style.css', 'health', 'session', 'catalog', 'events'].includes(resource) && ['GET', 'HEAD'].includes(method)) ||
    (['session', 'restore', 'resume', 'recovery-key'].includes(resource) && method === 'POST') ||
    (resource === 'session' && method === 'DELETE')
  );
  if (!allowed) throw createError({ statusCode: 400, statusMessage: "Invalid recovery request" });
  // fetch 会使用上游 Host；先在浏览器边界检查原始来源，再以匹配的上游来源转发。
  // 不能仅改写 Origin 而跳过这一层，否则会绕过后端的同源保护。
  const origin = getHeader(event, 'origin') || '';
  if (getHeader(event, 'sec-fetch-site') === 'cross-site' || (origin && origin !== url.origin)) throw createError({ statusCode: 403, statusMessage: 'Recovery requests must originate from this site' });
  const cfg = useRuntimeConfig(event);
  const legacy = job && process.env.POWERX_RECOVERY_STATUS_URL;
  const upstream = legacy || process.env.POWERX_BACKEND || cfg.upstream || 'http://127.0.0.1:8077';
  let target: URL;
  try { target = new URL(String(upstream)); } catch { throw createError({ statusCode: 503, statusMessage: "Invalid recovery backend" }); }
  if (!['http:', 'https:'].includes(target.protocol) || target.username || target.password || target.search || target.hash) throw createError({ statusCode: 503, statusMessage: "Invalid recovery backend" });
  if (legacy && (target.protocol !== 'http:' || target.hostname !== '127.0.0.1' || target.pathname !== '/')) throw createError({ statusCode: 503, statusMessage: "Invalid coordinator configuration" });
  const authorization = getHeader(event, "authorization") || "";
  if (job && !/^Bearer [0-9]{10}\.[0-9a-f]{64}$/.test(authorization)) throw createError({ statusCode: 401, statusMessage: "Progress authorization required" });
  const headers = { authorization, host: target.host, ...(origin ? { origin: target.origin } : {}) };
  setResponseHeader(event, "Cache-Control", "no-store");
  setResponseHeader(event, "X-Accel-Buffering", "no");
  if (url.pathname.endsWith("/events")) return proxyBackupStream(event, `${target.origin}${url.pathname}`, headers);
  return proxyRequest(event, `${target.origin}${url.pathname}`, { headers });
});
