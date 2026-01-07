import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { useKnowledgeSpaces } from "~/composables/useKnowledgeSpaces";

describe("useKnowledgeSpaces.triggerIngestion", () => {
  const originalRuntime = globalThis.useRuntimeConfig;

  beforeEach(() => {
    globalThis.useRuntimeConfig = () =>
      ({
        public: { apiBase: "/api" },
      }) as any;
  });

afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    if (originalRuntime) {
      globalThis.useRuntimeConfig = originalRuntime;
    } else {
      // eslint-disable-next-line @typescript-eslint/no-dynamic-delete
      delete (globalThis as any).useRuntimeConfig;
    }
  });

  it("posts payload to admin endpoint", async () => {
    const mockResponse = {
      data: {
        jobId: "job-1",
        status: "completed",
        retryCount: 0,
        chunkTotal: 9,
        chunkCoveragePct: 100,
        embeddingSuccessPct: 100,
        maskingCoveragePct: 100,
      },
    };
    const fetchSpy = vi.fn().mockResolvedValue(mockResponse);
    vi.stubGlobal("$fetch", fetchSpy);

    const api = useKnowledgeSpaces();
    const result = await api.triggerIngestion("space-1", {
      format: "pdf",
      sourceUri: "s3://bucket/doc.pdf",
      priority: "high",
    });
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/admin/knowledge-spaces/space-1/ingestion-jobs",
      {
        method: "POST",
        body: {
          format: "pdf",
          sourceUri: "s3://bucket/doc.pdf",
          priority: "high",
        },
      },
    );
    expect(result.jobId).toBe("job-1");
  });

  it("throws when spaceId missing", async () => {
    const api = useKnowledgeSpaces();
    await expect(
      api.triggerIngestion("", {
        format: "pdf",
        sourceUri: "s3://bucket/doc.pdf",
      }),
    ).rejects.toThrow("spaceId is required");
  });
});
