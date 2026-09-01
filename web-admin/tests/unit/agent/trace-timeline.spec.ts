import { describe, expect, it } from "vitest";
import type { AgentTraceEvent } from "~/composables/api/types/agentTrace";
import {
  buildAgentTraceTimeline,
  filterTraceEventsForRun,
} from "~/utils/agent/traceTimeline";

const event = (
  overrides: Partial<AgentTraceEvent>
): AgentTraceEvent => ({
  trace_id: "trace-1",
  run_id: "run-1",
  tenant_uuid: "tenant-1",
  agent_id: "agent-1",
  session_id: "session-1",
  message_id: "message-1",
  node_id: "node-1",
  node_seq: 1,
  node_kind: "receive_message",
  phase: "start",
  status: "running",
  created_at: "2026-08-28T01:00:00.000Z",
  ...overrides,
});

describe("agent trace timeline", () => {
  it("keeps only the explicitly selected run", () => {
    const events = [
      event({ run_id: "run-old" }),
      event({ run_id: "run-current", node_id: "current" }),
    ];

    expect(filterTraceEventsForRun(events, "run-current")).toHaveLength(1);
    expect(filterTraceEventsForRun(events, "")).toEqual([]);
  });

  it("merges node phases and orders nodes by actual start time", () => {
    const events = [
      event({
        node_id: "planner",
        node_seq: 2,
        node_kind: "planner",
        created_at: "2026-08-28T01:00:02.000Z",
      }),
      event({
        node_id: "receive",
        node_seq: 1,
        phase: "end",
        status: "success",
        duration_ms: 800,
        created_at: "2026-08-28T01:00:00.800Z",
      }),
      event({ node_id: "receive", node_seq: 1 }),
      event({
        node_id: "planner",
        node_seq: 2,
        node_kind: "planner",
        phase: "error",
        status: "error",
        duration_ms: 19,
        error_code: "SKILL_FAILED",
        error_summary: "execution failed",
        created_at: "2026-08-28T01:00:02.019Z",
      }),
    ];

    const timeline = buildAgentTraceTimeline(events);
    expect(timeline.map((item) => item.nodeId)).toEqual(["receive", "planner"]);
    expect(timeline[0]).toMatchObject({ status: "success", durationMs: 800 });
    expect(timeline[1]).toMatchObject({
      status: "error",
      durationMs: 19,
      errorCode: "SKILL_FAILED",
    });
  });
});
