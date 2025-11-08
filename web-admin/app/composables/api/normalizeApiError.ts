// utils/normalizeApiError.ts
export interface NormalizedApiError {
  title: string;
  description: string;
  fields: Record<string, string>;
  code?: number;
  raw?: any;
}

/**
 * 统一把后端错误转为可显示的标题/描述，以及字段级错误
 * - 默认兼容结构：
 *   { code, message, error, errors: { field: [msg]|msg } }
 */
export function normalizeApiError(
  err: any,
  fieldMap: Record<string, string> = { meta: "metaText" }
): NormalizedApiError {
  const res = err?.response;
  const data = res?._data ?? err?.data ?? err ?? {};
  const code = Number(res?.status ?? data?.code ?? 0);
  const backendMsg = String(data?.error || data?.message || err?.message || "");
  const fields: Record<string, string> = {};

  console.log("Normalize API error:", err, res, data);
  const rawFields = data?.errors || data?.fieldErrors;
  if (rawFields && typeof rawFields === "object") {
    for (const k of Object.keys(rawFields)) {
      const v = rawFields[k];
      const first = Array.isArray(v) ? v[0] : String(v);
      const fk = fieldMap[k] || k;
      fields[fk] = first;
    }
  }

  let title = "保存失败";
  let description = backendMsg || "操作失败，请稍后重试";

  // 你的示例：400 + invalid department key
  if (code === 400 && /invalid.*key/i.test(backendMsg)) {
    description =
      "Key 不合法：请用“小写字母开头”，只含小写字母、数字与中划线（如：marketing-center）。";
    if (!fields.key) fields.key = "Key 不合法";
  }

  console.log("Normalized API error:", { code, backendMsg, fields });

  if (code === 422 && Object.keys(fields).length) {
    const firstKey = Object.keys(fields)[0];
    description = fields[firstKey];
  }

  return { title, description, fields, code, raw: data };
}
