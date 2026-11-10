import { defineEventHandler, getRequestURL, setResponseHeader, createError } from "h3";
import { useRuntimeConfig } from "#imports";
import { challengeID, proofTarget, fetchPublicProof } from "../utils/marketplace-proof";

export default defineEventHandler(async (event) => {
  const url = getRequestURL(event);
  const cfg = useRuntimeConfig(event);
  const base = String(cfg.app?.baseURL || "/").replace(/\/+$/, "");
  const fixedPath = "/.well-known/powerx-instance.json";
  if (url.pathname !== `${base}${fixedPath}` && url.pathname !== fixedPath) return;
  setResponseHeader(event, "Cache-Control", "no-store");
  setResponseHeader(event, "Pragma", "no-cache");
  if (event.method !== "GET") throw createError({ statusCode: 405, message: "仅支持 GET" });
  const id = challengeID(url.search);
  let target: string;
  try { target = proofTarget(String(process.env.POWERX_BACKEND || cfg.upstream), String(cfg.public.apiBase || "/api/v1"), id); }
  catch { throw createError({ statusCode: 503, message: "证明上游暂不可用" }); }
  return fetchPublicProof(target, id);
});
