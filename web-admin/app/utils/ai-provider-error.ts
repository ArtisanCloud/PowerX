// 提取驱动保留的供应商错误；无法解析时交给原有摘要逻辑，完整详情仍由调用方保留。
export function aiProviderErrorMessage(raw: string): string | null {
  const bodyPos = raw.indexOf("body=");
  if (bodyPos < 0) return null;
  try {
    const body = JSON.parse(raw.slice(bodyPos + 5).trim());
    const message = body?.error?.message ?? body?.message;
    return typeof message === "string" && message.trim() ? message.trim() : null;
  } catch {
    return null;
  }
}
