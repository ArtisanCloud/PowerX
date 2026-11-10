// 只有正式登记的 App 前缀才是分隔符；Ollama 的 :14b/:latest 属于原始型号。
export function splitAIModelKey(raw: string | null | undefined, apps: { id: string }[] = []) {
  const value = String(raw || "").trim();
  const index = value.indexOf(":");
  const prefix = value.slice(0, index);
  if (index <= 0 || !apps.some(app => app.id.toLowerCase() === prefix.toLowerCase())) return { app: "", model: value };
  return { app: prefix, model: value.slice(index + 1) };
}
