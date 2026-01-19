import { describe, expect, it } from "vitest";
import { findEnableOcrRecommendation } from "~/utils/knowledge-spaces/recommendations";
import { buildIngestionRemediation } from "~/utils/knowledge-spaces/ingestionRemediation";

describe("knowledge-spaces OCR recommendation + remediation", () => {
  it("findEnableOcrRecommendation returns enable_ocr recommendation", () => {
    const rec = findEnableOcrRecommendation([
      { key: "scene_bundle", type: "scene_bundle" },
      { key: "enable_ocr", title: "扫描件占比偏高：建议启用 OCR", plugin: "com.powerx.plugin.data_forge" },
    ]);
    expect(rec?.key).toBe("enable_ocr");
    expect(rec?.pluginId).toBe("com.powerx.plugin.data_forge");
  });

  it("buildIngestionRemediation maps blocked ocr_required", () => {
    const r = buildIngestionRemediation({
      jobId: "job-1",
      status: "blocked",
      retryCount: 0,
      errorCode: "ocr_required",
      reason: "ocr_processor_unavailable",
      chunkTotal: 0,
      chunkCoveragePct: 0,
      embeddingSuccessPct: 0,
      maskingCoveragePct: 0,
    } as any);
    expect(r?.level).toBe("error");
    expect(r?.actions?.some((a) => a.kind === "link")).toBe(true);
  });

  it("buildIngestionRemediation maps degraded ocr_unavailable", () => {
    const r = buildIngestionRemediation({
      jobId: "job-1",
      status: "completed",
      retryCount: 0,
      errorCode: "degraded",
      reason: "ocr_unavailable",
      chunkTotal: 1,
      chunkCoveragePct: 40,
      embeddingSuccessPct: 100,
      maskingCoveragePct: 100,
    } as any);
    expect(r?.level).toBe("warning");
    expect(r?.actions?.some((a) => a.kind === "event")).toBe(true);
  });
});

