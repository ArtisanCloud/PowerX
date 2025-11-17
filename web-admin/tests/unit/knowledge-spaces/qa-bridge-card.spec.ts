import { describe, it, expect } from "vitest";
import { mount } from "@vue/test-utils";
import QaBridgeStatusCard from "../../../app/components/knowledge-spaces/QaBridgeStatusCard.vue";

describe("QaBridgeStatusCard", () => {
  const baseStatus = {
    latencyMsP95: 1200,
    citationCoverage: 0.96,
    toolSuccessRate: 0.99,
    degradeCount: 0,
    lastAuditId: "audit-123",
    lastUpdatedAt: "2025-01-02T12:00:00Z",
  };

  it("renders metrics and audit link", () => {
    const wrapper = mount(QaBridgeStatusCard, {
      props: { status: baseStatus },
    });
    expect(wrapper.find('[data-test="latency"]').text()).toContain("1.2s");
    expect(wrapper.find('[data-test="coverage"]').text()).toContain("96%");
    expect(wrapper.find('[data-test="tool-success"]').text()).toContain("99%");
    expect(wrapper.find('[data-test="audit-link"]').text()).toContain(
      "audit-123",
    );
  });

  it("shows degrade badge and emits refresh", async () => {
    const wrapper = mount(QaBridgeStatusCard, {
      props: {
        status: {
          ...baseStatus,
          degradeCount: 2,
        },
      },
    });
    expect(wrapper.find('[data-test="degrade-badge"]').exists()).toBe(true);
    await wrapper.find("button").trigger("click");
    expect(wrapper.emitted("refresh")).toBeTruthy();
  });
});
