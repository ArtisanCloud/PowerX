import { describe, it, expect } from "vitest";
import { formatEmbeddingGuardMessage, isEmbeddingProfileReady, resolveEmbeddingDimensions } from "~/utils/knowledge-spaces/embeddingGate";

describe("knowledge-spaces embedding gate", () => {
  it("returns false for missing profile", () => {
    expect(isEmbeddingProfileReady(null)).toBe(false);
    expect(isEmbeddingProfileReady({ configured: false })).toBe(false);
    expect(isEmbeddingProfileReady({ profile: null })).toBe(false);
  });

  it("detects dimensions from defaults/capCache", () => {
    expect(resolveEmbeddingDimensions({ defaults: { dimensions: 1536 } })).toBe(1536);
    expect(resolveEmbeddingDimensions({ defaults: { dim: "1024" } })).toBe(1024);
    expect(resolveEmbeddingDimensions({ capCache: { dimensions: 768 } })).toBe(768);
  });

  it("returns true when a valid dimension exists", () => {
    expect(
      isEmbeddingProfileReady({
        configured: true,
        profile: { defaults: { dimensions: 1536 }, capCache: { probed_at: "now" } },
      }),
    ).toBe(true);
    expect(isEmbeddingProfileReady({ profile: { capCache: { dimensions: 768, probed_at: "ok" } } })).toBe(true);
  });

  it("formats provider/model aware message", () => {
    const msg = formatEmbeddingGuardMessage({ profile: { provider: "huggingface", model: "bge-small" } });
    expect(msg).toContain("huggingface/bge-small");
  });
});
