export interface AgentTraceEvent {
  event_id?: string;
  trace_id: string;
  run_id: string;
  tenant_uuid: string;
  user_uuid?: string;
  agent_id: string;
  session_id: string;
  message_id: string;
  plan_id?: string;
  node_id: string;
  node_seq: number;
  node_kind: string;
  node_ref?: string;
  phase: string;
  status: string;
  duration_ms?: number;
  input_digest?: string;
  output_digest?: string;
  error_code?: string;
  error_summary?: string;
  attributes?: Record<string, unknown>;
  created_at: string;
}

export interface AgentTraceNode {
  node_id: string;
  node_seq: number;
  node_kind: string;
  node_ref?: string;
  phase_status: string;
  input_summary?: Record<string, unknown>;
  output_summary?: Record<string, unknown>;
  context_ref?: string;
  skill_id?: string;
  plugin_id?: string;
  capability_id?: string;
  executor_path?: string;
  error_code?: string;
  error_summary?: string;
  started_at?: string;
  ended_at?: string;
}

export interface AgentTraceReport {
  report_scope: string;
  format: string;
  tenant_uuid: string;
  session_id: string;
  message_id?: string;
  run_id: string;
  trace_id: string;
  generated_by?: string;
  generated_at: string;
  summary?: Record<string, unknown>;
  timeline?: AgentTraceEvent[];
  nodes?: AgentTraceNode[];
  errors?: Array<Record<string, unknown>>;
  artifact_refs?: string[];
}

export interface AgentTraceTimelineResult {
  items: AgentTraceEvent[];
  nodes: AgentTraceNode[];
  tenant_uuid: string;
  session_id: string;
  message_id: string;
}

export interface AgentTraceQuery {
  tenant_uuid: string;
  session_id: string;
  message_id: string;
  run_id?: string;
  trace_id?: string;
}
