import type { AgentTraceEvent } from "~/composables/api/types/agentTrace";

export type AgentTraceTimelineStatus = "running" | "success" | "error";

export interface AgentTraceTimelineItem {
  key: string;
  runId: string;
  nodeId: string;
  nodeSeq: number;
  nodeKind: string;
  nodeRef: string;
  status: AgentTraceTimelineStatus;
  startedAt: string;
  endedAt: string;
  durationMs: number;
  errorCode: string;
  errorSummary: string;
}

export const filterTraceEventsForRun = (
  events: ReadonlyArray<AgentTraceEvent>,
  runId: string
): AgentTraceEvent[] => {
  const normalizedRunId = String(runId || "").trim();
  if (!normalizedRunId) return [];
  return events.filter(
    (event) => String(event.run_id || "").trim() === normalizedRunId
  );
};

export const buildAgentTraceTimeline = (
  events: ReadonlyArray<AgentTraceEvent>
): AgentTraceTimelineItem[] => {
  const groups = new Map<string, AgentTraceEvent[]>();
  for (const event of events) {
    const runId = String(event.run_id || "").trim();
    const nodeId = String(event.node_id || "").trim();
    if (!runId || !nodeId) continue;
    const key = `${runId}:${nodeId}`;
    const group = groups.get(key) || [];
    group.push(event);
    groups.set(key, group);
  }

  const items = Array.from(groups.entries()).map(([key, group]) => {
    const ordered = [...group].sort(compareEventTime);
    const first = ordered[0];
    const start = ordered.find((event) => event.phase === "start") || first;
    const error = ordered.findLast(
      (event) => event.phase === "error" || event.status === "error"
    );
    const end = ordered.findLast((event) => event.phase === "end");
    const terminal = error || end;
    const status: AgentTraceTimelineStatus = error
      ? "error"
      : end
        ? "success"
        : "running";
    const explicitDuration = Number(terminal?.duration_ms);
    const calculatedDuration = terminal
      ? Math.max(
          0,
          new Date(terminal.created_at).getTime() -
            new Date(start.created_at).getTime()
        )
      : 0;
    const sequences = ordered
      .map((event) => Number(event.node_seq || 0))
      .filter((value) => value > 0);

    return {
      key,
      runId: String(first.run_id || ""),
      nodeId: String(first.node_id || ""),
      nodeSeq: sequences.length ? Math.min(...sequences) : 0,
      nodeKind: String(first.node_kind || ""),
      nodeRef: String(first.node_ref || first.node_id || ""),
      status,
      startedAt: start.created_at,
      endedAt: terminal?.created_at || "",
      durationMs:
        Number.isFinite(explicitDuration) && explicitDuration >= 0
          ? explicitDuration
          : calculatedDuration,
      errorCode: String(error?.error_code || ""),
      errorSummary: String(error?.error_summary || ""),
    };
  });

  return items.sort((left, right) => {
    const timeDiff =
      new Date(left.startedAt).getTime() - new Date(right.startedAt).getTime();
    if (timeDiff !== 0) return timeDiff;
    if (left.nodeSeq !== right.nodeSeq) return left.nodeSeq - right.nodeSeq;
    return left.nodeId.localeCompare(right.nodeId);
  });
};

const compareEventTime = (left: AgentTraceEvent, right: AgentTraceEvent) => {
  const timeDiff =
    new Date(left.created_at).getTime() - new Date(right.created_at).getTime();
  if (timeDiff !== 0) return timeDiff;
  return Number(left.node_seq || 0) - Number(right.node_seq || 0);
};
