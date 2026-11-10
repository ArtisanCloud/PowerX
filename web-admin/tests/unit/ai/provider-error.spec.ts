import { describe, expect, it } from "vitest";
import { aiProviderErrorMessage } from "~/utils/ai-provider-error";

describe("供应商错误摘要", () => {
  it.each([
    "Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.",
    "Unsupported value: 'temperature' does not support 0.7 with this model. Only the default (1) value is supported.",
  ])("直接显示供应商给出的参数原因：%s", message => {
    const raw = `openai invoke url=https://api.openai.com/v1/chat/completions status=400 body=${JSON.stringify({ error: { message, param: "temperature" } })}`;
    expect(aiProviderErrorMessage(raw)).toBe(message);
  });

  it.each(["connection refused", "status=502 body=<html>bad gateway</html>", 'status=400 body={"error":{}}'])
    ("非 JSON 或不含原因的响应保留原摘要回退：%s", raw => {
      expect(aiProviderErrorMessage(raw)).toBeNull();
    });
});
