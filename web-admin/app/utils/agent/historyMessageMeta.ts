export function normalizeHistoryMessageMeta(meta: Record<string, any>): Record<string, any> {
  const normalized = { ...(meta || {}) };
  if (normalized.run_state && !normalized.runState) {
    normalized.runState = normalized.run_state;
  }
  if (normalized.pending_task && !normalized.pendingTask) {
    normalized.pendingTask = normalized.pending_task;
  }
  if (normalized.response_envelope && !normalized.responseEnvelope) {
    normalized.responseEnvelope = normalized.response_envelope;
  }
  if (normalized.response_labels) normalized.responseLabels = normalized.response_labels;
  if (normalized.response_evidence_state) normalized.responseEvidenceState = normalized.response_evidence_state;
  if (normalized.failure_stage) normalized.failureStage = normalized.failure_stage;
  return normalized;
}
