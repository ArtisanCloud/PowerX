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
  return normalized;
}
