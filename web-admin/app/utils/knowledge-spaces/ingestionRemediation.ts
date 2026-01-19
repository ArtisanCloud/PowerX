import type { IngestionJobRecord } from "~/composables/useKnowledgeSpaces";

export type IngestionRemediationAction =
  | { kind: "link"; label: string; to: string }
  | { kind: "event"; label: string; event: "enable_ocr" | "disable_ocr" };

export type IngestionRemediation = {
  level: "error" | "warning";
  title: string;
  description: string;
  actions?: IngestionRemediationAction[];
};

const OCR_PLUGIN_ID = "com.powerx.plugin.data_forge";

export function buildIngestionRemediation(job: IngestionJobRecord | null | undefined): IngestionRemediation | null {
  if (!job) return null;

  if (job.status === "blocked") {
    if (job.errorCode === "ocr_required") {
      return {
        level: "error",
        title: "入库被阻止：需要 OCR 能力",
        description: "当前文档需要 OCR，但后端未提供 OCR 处理器。请先安装 OCR 扩展，或关闭“OCR required”。",
        actions: [
          { kind: "link", label: "去安装 OCR 插件", to: `/plugins/market?pluginId=${encodeURIComponent(OCR_PLUGIN_ID)}` },
          { kind: "event", label: "关闭 OCR required", event: "disable_ocr" },
        ],
      };
    }
    if (job.errorCode === "masking_required") {
      return {
        level: "error",
        title: "入库被阻止：脱敏未通过",
        description: "本次入库命中脱敏阻断策略。请切换/配置脱敏策略，或先对源文档做脱敏预处理。",
      };
    }
    return {
      level: "error",
      title: "入库被阻止",
      description: job.reason ? `${job.errorCode || "blocked"}：${job.reason}` : (job.errorCode || "blocked"),
    };
  }

  if (job.errorCode === "degraded") {
    if (job.reason === "ocr_unavailable") {
      return {
        level: "warning",
        title: "入库已降级：OCR 不可用",
        description: "检测到可能需要 OCR，但当前未启用 OCR 能力；本次入库内容可能为空或引用覆盖下降。",
        actions: [
          { kind: "link", label: "去安装 OCR 插件", to: `/plugins/market?pluginId=${encodeURIComponent(OCR_PLUGIN_ID)}` },
          { kind: "event", label: "下次入库启用 OCR", event: "enable_ocr" },
        ],
      };
    }
    if (job.reason === "processor_profile_unavailable") {
      return {
        level: "warning",
        title: "入库已降级：Processor Profile 不可用",
        description: "指定的 processorProfile 后端不支持，已回退到默认处理器；建议检查 processorProfile 配置或安装对应插件。",
      };
    }
    if (job.reason === "empty_content") {
      return {
        level: "warning",
        title: "入库已降级：内容为空",
        description: "解析未提取到有效内容，已使用占位 chunk。通常是 OCR/解析器缺失或源链接不可访问导致。",
      };
    }
    return {
      level: "warning",
      title: "入库已降级",
      description: job.reason ? `${job.errorCode}：${job.reason}` : job.errorCode,
    };
  }

  return null;
}

