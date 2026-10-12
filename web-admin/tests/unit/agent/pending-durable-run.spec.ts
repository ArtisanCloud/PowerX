import { beforeEach, describe, expect, it } from 'vitest'
import { durableResumeParams, historyHasDurableResult, pendingDurableRuns, removePendingDurableRun, savePendingDurableRun } from '~/utils/agent/pendingDurableRun'

const runId = '2a07c49a-02fb-45ef-b9ee-af6aa23ba132'
const ticket = () => ({ runId, agentId: 'marketing-lead', sessionId: '42', updatedAt: Date.now() })

describe('accepted Run refresh recovery', () => {
  beforeEach(() => localStorage.clear())
  it('restores one accepted Run after page reconstruction and never resubmits input', () => {
    savePendingDurableRun(localStorage, 'user:tenant:dev', { ...ticket(), input: 'private campaign', token: 'private JWT' } as any)
    const serialized = Object.values(localStorage).join('')
    expect(serialized).not.toContain('private')
    const restored = pendingDurableRuns(localStorage, 'user:tenant:dev')[0]!
    expect(restored.runId).toBe(runId)
    const params = durableResumeParams(restored.runId, 'dev')
    expect(params).toEqual({ env: 'dev', run_id: runId, after_seq: '0' })
    expect(params).not.toHaveProperty('q')
    expect(params).not.toHaveProperty('client_msg_id')
    expect(historyHasDurableResult([{ role: 'assistant', isStreaming: true, meta: { trace: { run_id: runId } } }], runId)).toBe(false)
    expect(historyHasDurableResult([{ role: 'assistant', meta: { trace: { run_id: runId } } }], runId)).toBe(true)
    removePendingDurableRun(localStorage, 'user:tenant:dev', runId)
    expect(pendingDurableRuns(localStorage, 'user:tenant:dev')).toEqual([])
  })
  it('isolates users, tenants and environments and preserves other sessions', () => {
    savePendingDurableRun(localStorage, 'user:tenant:dev', ticket())
    savePendingDurableRun(localStorage, 'user:tenant:dev', { ...ticket(), runId: 'daff2131-a252-4ef8-beb8-1be3d41d0e60', sessionId: '43' })
    expect(pendingDurableRuns(localStorage, 'user:tenant:dev')).toHaveLength(2)
    for (const scope of ['other:tenant:dev', 'user:other:dev', 'user:tenant:prod']) expect(pendingDurableRuns(localStorage, scope)).toEqual([])
    removePendingDurableRun(localStorage, 'user:tenant:dev', runId)
    expect(pendingDurableRuns(localStorage, 'user:tenant:dev')[0]?.sessionId).toBe('43')
  })
  it('bounds stale checkpoints and rejects malformed or unavailable storage', () => {
    savePendingDurableRun(localStorage, 'user:tenant:dev', { ...ticket(), updatedAt: Date.now() - 8 * 86400000 })
    expect(pendingDurableRuns(localStorage, 'user:tenant:dev')).toEqual([])
    expect(() => durableResumeParams('not-a-run', 'dev')).toThrow()
    const unavailable = { getItem() { throw new Error('disabled') }, setItem() { throw new Error('disabled') }, removeItem() { throw new Error('disabled') } }
    expect(() => savePendingDurableRun(unavailable, 'user:tenant:dev', ticket())).not.toThrow()
    expect(pendingDurableRuns(unavailable, 'user:tenant:dev')).toEqual([])
  })
})
