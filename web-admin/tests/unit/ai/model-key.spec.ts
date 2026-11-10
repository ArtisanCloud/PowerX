import { describe, expect, it } from "vitest";
import { splitAIModelKey } from "~/utils/ai-model-key";

describe("模型原始标识与 App 前缀", () => {
  it.each(["qwen3:14b", "qwen3.5:cloud", "bge-m3:latest", "registry.local/team/model:latest"])("保留 Ollama 原生型号 %s", model => {
    expect(splitAIModelKey(model)).toEqual({ app: "", model });
  });
  it("仅拆分已登记的 App，保留型号自身的冒号", () => {
    expect(splitAIModelKey("vision:custom:latest", [{ id: "vision" }])).toEqual({ app: "vision", model: "custom:latest" });
    expect(splitAIModelKey("qwen3:14b", [{ id: "vision" }])).toEqual({ app: "", model: "qwen3:14b" });
  });
});
