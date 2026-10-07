import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, ref, type App } from 'vue'
import { useDualChannelConnection, type DualChannelConnection } from '~/composables/agent/useDualChannelConnection'
import { pendingDurableRuns, savePendingDurableRun } from '~/utils/agent/pendingDurableRun'

vi.mock('~/stores/envStore', () => ({ useEnvStore: () => ({ currentEnv: 'dev' }) }))
vi.mock('~/stores/user', () => ({ useUserStore: () => ({ user: { id: 1 }, currentTenantUuid: 'tenant' }) }))
vi.mock('~/stores/message', () => ({ useMessageStore: () => ({ setMessages: vi.fn(), getMessagesBySession: () => [] }) }))

const runId = '2a07c49a-02fb-45ef-b9ee-af6aa23ba132'
let app: App | undefined
let chat: DualChannelConnection
const host = document.createElement('div')
beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('access_token', 'test-only-token')
  localStorage.setItem('px_current_tenant_uuid', 'tenant')
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { apiBase: 'http://core/api/v1' } }))
  vi.stubGlobal('useI18n', () => ({ t: (key: string) => key }))
  vi.stubGlobal('WebSocket', class { constructor() { throw new Error('probe disabled in unit test') } })
})
afterEach(() => { app?.unmount(); app = undefined; vi.unstubAllGlobals() })
const mount = () => {
  app = createApp(defineComponent({ setup() { chat = useDualChannelConnection(ref('marketing'), ref('42')); return () => null } }))
  app.mount(host)
}

it('rebuilds from an accepted checkpoint, resumes without q, and renders exactly one result', async () => {
  savePendingDurableRun(localStorage, '1:tenant:dev', { runId, agentId: 'marketing', sessionId: '42', updatedAt: Date.now() })
  const requests: URL[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    const parsed = new URL(url); requests.push(parsed)
    const body = parsed.searchParams.has('probe') ? ': connected\n\n' :
      `event: meta\ndata: ${JSON.stringify({ durable_run: true, run_id: runId, session_id: '42', message_id: '9' })}\n\nid: 1\nevent: agent_run.task_status\ndata: ${JSON.stringify({ run_id: runId, payload: { task_id: 'analysis', status: 'completed' } })}\n\nevent: final\ndata: ${JSON.stringify({ success: true, data: { content: '营销复盘结果' } })}\n\n`
    return new Response(new ReadableStream({ start(controller) { controller.enqueue(new TextEncoder().encode(body)); controller.close() } }), { status: 200 })
  }))
  mount()
  chat.messages.value = [{ id: 9, role: 'user', content: '历史输入', done: true }]
  await chat.resumePendingRun()
  const resumed = requests.filter((url) => !url.searchParams.has('probe'))
  expect(resumed).toHaveLength(1)
  expect(resumed[0]!.searchParams.get('run_id')).toBe(runId)
  expect(resumed[0]!.searchParams.has('q')).toBe(false)
  expect(resumed[0]!.searchParams.has('client_msg_id')).toBe(false)
  expect(chat.messages.value.filter((message) => message.role === 'user')).toHaveLength(1)
  const answers = chat.messages.value.filter((message) => message.role === 'assistant')
  expect(answers).toHaveLength(1)
  expect(answers[0].content).toBe('营销复盘结果')
  expect(chat.isGenerating.value).toBe(false)
  expect(pendingDurableRuns(localStorage, '1:tenant:dev')).toEqual([])
})

it('uses an already persisted final message without creating another subscription', async () => {
  savePendingDurableRun(localStorage, '1:tenant:dev', { runId, agentId: 'marketing', sessionId: '42', updatedAt: Date.now() })
  const fetchMock = vi.fn(async () => new Response(': connected\n\n'))
  vi.stubGlobal('fetch', fetchMock)
  mount()
  chat.messages.value = [{ id: 10, role: 'assistant', content: '已完成', meta: { run_id: runId } }]
  await chat.resumePendingRun()
  expect(fetchMock.mock.calls.filter(([url]: any[]) => !new URL(url).searchParams.has('probe'))).toHaveLength(0)
  expect(chat.messages.value).toHaveLength(1)
  expect(pendingDurableRuns(localStorage, '1:tenant:dev')).toEqual([])
})

it('creates a distinct admission key on each explicit retry and replaces the old Run trace', async () => {
  const requests: URL[] = []
  let attempts = 0
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    const parsed = new URL(url)
    if (parsed.searchParams.has('probe')) return new Response(': connected\n\n')
    requests.push(parsed)
    attempts++
    const nextRun = attempts === 1 ? runId : '4acb2efc-c4b9-4fba-9027-6577bcc4aeb9'
    const body = `event: meta\ndata: ${JSON.stringify({ durable_run: true, run_id: nextRun, session_id: '42', message_id: '9' })}\n\nevent: final\ndata: ${JSON.stringify({ success: true, data: { content: '本次结果' } })}\n\n`
    return new Response(new ReadableStream({ start(controller) { controller.enqueue(new TextEncoder().encode(body)); controller.close() } }))
  }))
  mount()
  chat.messages.value = [
    { id: 9, role: 'user', content: '复盘原问题', done: true },
    { id: 10, role: 'assistant', content: '旧失败', isError: true, meta: { trace: { run_id: 'old-run', trace_id: 'old-trace' } } },
  ]
  await chat.regenerateFrom(9, 'chat', undefined, { team_id: '3', client_msg_id: 'old-key' })
  await chat.regenerateFrom(9, 'chat', undefined, { team_id: '3', client_msg_id: 'old-key' })
  expect(requests).toHaveLength(2)
  const keys = requests.map(url => url.searchParams.get('client_msg_id'))
  expect(keys[0]).toMatch(/^retry_/)
  expect(keys[1]).toMatch(/^retry_/)
  expect(keys[0]).not.toBe(keys[1])
  for (const url of requests) {
    expect(url.searchParams.get('regen_from_message_id')).toBe('9')
    expect(url.searchParams.get('team_id')).toBe('3')
    expect(url.searchParams.has('run_id')).toBe(false)
  }
  expect(chat.messages.value.filter(message => message.role === 'user')).toHaveLength(1)
  const answers = chat.messages.value.filter(message => message.role === 'assistant')
  expect(answers).toHaveLength(1)
  expect(answers[0].meta?.trace?.run_id).toBe('4acb2efc-c4b9-4fba-9027-6577bcc4aeb9')
  expect(answers[0].meta?.trace?.trace_id).not.toBe('old-trace')
  expect(chat.isGenerating.value).toBe(false)
})

it('stops thinking and clears the checkpoint when a failed historical Run has no final message', async () => {
  savePendingDurableRun(localStorage, '1:tenant:dev', { runId, agentId: 'marketing', sessionId: '42', updatedAt: Date.now() })
  const requests: URL[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    const parsed = new URL(url)
    if (parsed.searchParams.has('probe')) return new Response(': connected\n\n')
    requests.push(parsed)
    const body = `event: meta\ndata: ${JSON.stringify({ durable_run: true, run_id: runId, session_id: '42' })}\n\nevent: agent_run.task_status\ndata: ${JSON.stringify({ run_id: runId, payload: { task_id: 'plan', status: 'failed' } })}\n\nevent: agent_run.ended\ndata: ${JSON.stringify({ run_id: runId, payload: { status: 'failed', success: false } })}\n\nevent: end\ndata: {"success":false}\n\n`
    return new Response(new ReadableStream({ start(controller) { controller.enqueue(new TextEncoder().encode(body)); controller.close() } }))
  }))
  mount()
  chat.messages.value = [{ id: 9, role: 'user', content: '原问题', done: true }]
  await chat.resumePendingRun()
  expect(requests).toHaveLength(1)
  expect(chat.isGenerating.value).toBe(false)
  const answer = chat.messages.value.find(message => message.role === 'assistant')!
  expect(answer.isThinking).toBe(false)
  expect(answer.isStreaming).toBe(false)
  expect(answer.done).toBe(true)
  expect(answer.isError).toBe(true)
  expect(answer.meta?.runState?.ended).toBe(true)
  expect(pendingDurableRuns(localStorage, '1:tenant:dev')).toEqual([])
  await chat.resumePendingRun()
  expect(requests).toHaveLength(1)
})
