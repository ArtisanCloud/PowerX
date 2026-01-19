export type EnableOcrRecommendation = {
  key: string;
  title?: string;
  pluginId?: string;
  reason?: Record<string, any>;
  risk?: string;
  cost?: string;
};

export function findEnableOcrRecommendation(recommendations: any[] | null | undefined): EnableOcrRecommendation | null {
  if (!Array.isArray(recommendations)) return null;
  const rec = recommendations.find((r) => r && (r.key === "enable_ocr" || r.type === "enable_ocr"));
  if (!rec) return null;
  return {
    key: String(rec.key || "enable_ocr"),
    title: typeof rec.title === "string" ? rec.title : undefined,
    pluginId: typeof rec.plugin === "string" ? rec.plugin : undefined,
    reason: rec.reason && typeof rec.reason === "object" ? rec.reason : undefined,
    risk: typeof rec.risk === "string" ? rec.risk : undefined,
    cost: typeof rec.cost === "string" ? rec.cost : undefined,
  };
}

