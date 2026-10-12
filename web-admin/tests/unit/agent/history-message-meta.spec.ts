import { describe, expect, it } from "vitest";
import { normalizeHistoryMessageMeta } from "~/utils/agent/historyMessageMeta";

describe("历史消息元数据", () => {
  it("将 V3 响应契约转换为实时渲染字段", () => {
    const envelope = {
      schema: "powerx.agent.response/v3",
      kind: "multi_agent_summary",
      outcome: "completed",
      presentation: {},
    };
    const meta = normalizeHistoryMessageMeta({ response_envelope: envelope });

    expect(meta.responseEnvelope).toBe(envelope);
  });

  it("不覆盖已经存在的前端字段", () => {
    const current = { schema: "powerx.agent.response/v3", presentation: {} };
    const legacy = { schema: "powerx.agent.response/v3", presentation: { facts: [] } };
    const meta = normalizeHistoryMessageMeta({
      responseEnvelope: current,
      response_envelope: legacy,
    });

    expect(meta.responseEnvelope).toBe(current);
  });
});
