export function isRunStateEvent(type: string): boolean {
  return type.startsWith('agent_run.')
}

// Run-state events describe orchestration only. Even agent_run.final can be
// emitted by a child Skill and is never eligible to replace the chat answer.
export function isVisibleAssistantContentEvent(type: string): boolean {
  return !isRunStateEvent(type)
}
