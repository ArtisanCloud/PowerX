import { createError } from "h3";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
export function challengeID(search: string): string {
  const query = new URLSearchParams(search);
  const id = query.get("challenge_id") || "";
  if ([...query.keys()].some(key => key !== "challenge_id") || query.getAll("challenge_id").length !== 1 || !uuid.test(id) || id === "00000000-0000-0000-0000-000000000000") {
    throw createError({ statusCode: 400, message: "challenge_id 无效" });
  }
  return id;
}

// 地址只能取服务端配置。浏览器的 Host、Cookie、Authorization 和其他查询不进入上游。
export function proofTarget(upstream: string, apiBase: string, id: string): string {
  const target = new URL(upstream);
  if (!["http:", "https:"].includes(target.protocol) || target.username || target.password || target.search || target.hash) throw new Error("Core 上游配置无效");
  const prefix = target.pathname !== "/" ? target.pathname.replace(/\/+$/, "") : apiBase.replace(/\/+$/, "");
  if (!/^\/[a-zA-Z0-9/_-]+$/.test(prefix) || prefix.includes("//")) throw new Error("Core API 前缀无效");
  target.pathname = `${prefix}/system/marketplace-proof`;
  target.searchParams.set("challenge_id", id);
  return target.href;
}

export async function fetchPublicProof(target: string, id: string) {
  let response: Response;
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 5000);
  try {
    response = await fetch(target, { method: "GET", headers: { Accept: "application/json" }, redirect: "error", cache: "no-store", signal: controller.signal });
    if (response.status !== 200) throw createError({ statusCode: response.status === 404 ? 404 : 503, message: "指定证明不可用" });
    if (!response.headers.get("content-type")?.toLowerCase().startsWith("application/json")) throw new Error("无效证明响应");
    const reader = response.body?.getReader();
    if (!reader) throw new Error("证明响应为空");
    const chunks: Uint8Array[] = [];
    let size = 0;
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.length;
      if (size > 16384) { await reader.cancel(); throw new Error("证明响应过大"); }
      chunks.push(value);
    }
    const envelope = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    const proof = envelope.data;
    if (envelope.code !== 200 || !proof || Object.keys(proof).sort().join(",") !== "challenge_id,expires_at,nonce,schema" || proof.schema !== "powerx.instance-proof/v1" || proof.challenge_id !== id || typeof proof.nonce !== "string" || !/^[A-Za-z0-9_-]{43}$/.test(proof.nonce) || Buffer.from(proof.nonce, "base64url").toString("base64url") !== proof.nonce || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$/.test(proof.expires_at) || !Number.isFinite(Date.parse(proof.expires_at))) throw new Error("无效证明响应");
    return { schema: proof.schema, challenge_id: proof.challenge_id, nonce: proof.nonce, expires_at: proof.expires_at };
  } catch (error) {
    if (error && typeof error === "object" && "statusCode" in error) throw error;
    throw createError({ statusCode: 503, message: "证明服务暂不可用" });
  } finally {
    clearTimeout(timeout);
    controller.abort();
  }
}
