// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { challengeID, proofTarget, fetchPublicProof } from "../../../server/utils/marketplace-proof";

const id = "12345678-1234-1234-1234-123456789abc";
const proof = { schema: "powerx.instance-proof/v1", challenge_id: id, nonce: "A".repeat(43), expires_at: "2026-01-01T00:10:00Z" };
afterEach(() => vi.unstubAllGlobals());
describe("固定证明代理", () => {
  it("仅接受指定挑战并拒绝任意目标、重复参数和枚举", () => {
    expect(challengeID(`?challenge_id=${id}`)).toBe(id);
    for (const query of ["", `?challenge_id=${id}&target=https://evil.example`, `?challenge_id=${id}&challenge_id=${id}`]) expect(() => challengeID(query)).toThrow();
    expect(proofTarget("http://core:8080", "/custom/v2", id)).toBe(`http://core:8080/custom/v2/system/marketplace-proof?challenge_id=${id}`);
    expect(proofTarget("http://core:8080/api/v2", "/api/v1", id)).toBe(`http://core:8080/api/v2/system/marketplace-proof?challenge_id=${id}`);
    expect(() => proofTarget("https://secret@evil.example", "/api/v1", id)).toThrow();
  });
  it("解出 Core envelope，且请求不附带访客凭证", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 200, data: proof }), { headers: { "content-type": "application/json" } }));
    vi.stubGlobal("fetch", fetch);
    expect(await fetchPublicProof("http://core/api/v1/system/marketplace-proof", id)).toEqual(proof);
    const options = fetch.mock.calls[0][1];
    expect(options.headers).toEqual({ Accept: "application/json" });
    expect(options.redirect).toBe("error");expect(options.cache).toBe("no-store");
  });
  it("拒绝多余私有字段、错配挑战、超大响应与失效上游", async () => {
    for (const data of [{ ...proof, instance_id: "private" }, { ...proof, challenge_id: "different" }, { ...proof, nonce: "bad" }]) {
      vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 200, data }), { headers: { "content-type": "application/json" } })));
      await expect(fetchPublicProof("http://core", id)).rejects.toMatchObject({ statusCode: 503 });
    }
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("x".repeat(16385), { headers: { "content-type": "application/json" } })));
    await expect(fetchPublicProof("http://core", id)).rejects.toMatchObject({ statusCode: 503 });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("private error", { status: 404 })));
    await expect(fetchPublicProof("http://core", id)).rejects.toMatchObject({ statusCode: 404 });
  });
});
