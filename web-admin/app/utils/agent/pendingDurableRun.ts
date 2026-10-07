/** 浏览器只保存续订定位，不保存输入、报告或凭据。后端始终重新鉴权。 */
export interface PendingDurableRun {
  runId: string
  agentId: string
  sessionId: string
  updatedAt: number
}
type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>
const ttl = 7 * 24 * 60 * 60 * 1000
const keyFor = (scope: string) => `powerx:pending-agent-runs:v1:${encodeURIComponent(scope)}`
const valid = (value: any, now: number): value is PendingDurableRun =>
  !!value && /^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(value.runId) &&
  typeof value.agentId === 'string' && value.agentId.length > 0 && value.agentId.length <= 128 &&
  typeof value.sessionId === 'string' && value.sessionId.length > 0 && value.sessionId.length <= 128 &&
  Number.isFinite(value.updatedAt) && value.updatedAt <= now && now - value.updatedAt < ttl

export function pendingDurableRuns(storage: StorageLike, scope: string, now = Date.now()): PendingDurableRun[] {
  if (!scope) return []
  try {
    const raw = JSON.parse(storage.getItem(keyFor(scope)) || '[]')
    if (!Array.isArray(raw)) return []
    return raw.filter((value) => valid(value, now)).map(({ runId, agentId, sessionId, updatedAt }) => ({ runId, agentId, sessionId, updatedAt })).sort((a, b) => b.updatedAt - a.updatedAt).slice(0, 20)
  } catch { return [] }
}
export function savePendingDurableRun(storage: StorageLike, scope: string, run: PendingDurableRun) {
  if (!scope || !valid(run, Date.now())) return
  const rows = pendingDurableRuns(storage, scope).filter((row) => row.sessionId !== run.sessionId)
  // 显式字段白名单，调用方的额外属性不能落盘。
  const { runId, agentId, sessionId, updatedAt } = run
  try { storage.setItem(keyFor(scope), JSON.stringify([{ runId, agentId, sessionId, updatedAt }, ...rows].slice(0, 20))) } catch { /* 存储禁用时仍可在当前页面续订。 */ }
}
export function removePendingDurableRun(storage: StorageLike, scope: string, runId: string) {
  if (!scope) return
  try {
    const rows = pendingDurableRuns(storage, scope).filter((row) => row.runId !== runId)
    if (rows.length) storage.setItem(keyFor(scope), JSON.stringify(rows))
    else storage.removeItem(keyFor(scope))
  } catch { /* 存储不可用不影响已持久化的运行。 */ }
}
export function durableResumeParams(runId: string, env: string) {
  if (!/^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(runId)) throw new Error('Invalid durable Run ID')
  // 页面重建从零重放状态，避免仅保存游标却丢失未保存的任务视图。
  return { env, run_id: runId, after_seq: '0' }
}
export function historyHasDurableResult(messages: any[], runId: string) {
  return messages.some((message) => message.role === 'assistant' && !message.isStreaming &&
    String(message.meta?.trace?.run_id || message.meta?.runState?.run?.run_id || message.meta?.run_id || '') === runId)
}
