import { describe, expect, it, vi } from 'vitest'
import { durableSseReader } from '../../../app/utils/agent/durableSseReader'

const encode = (text: string) => new TextEncoder().encode(text)
const reader = (...chunks: string[]) => new ReadableStream<Uint8Array>({ start(controller) { chunks.forEach(s => controller.enqueue(encode(s))); controller.close() } }).getReader()
const id = '72d60d29-758f-4a51-a9ce-fc16a8d24a7e'
const meta = `event: meta\ndata: {"run_id":"${id}","durable_run":true}\n\n`
const final = 'event: final\ndata: {"success":true}\n\n'

describe('durable Run SSE recovery', () => {
  it('resumes the same Run with the last complete event and discards a split failed frame', async () => {
    const reconnect = vi.fn(async () => reader('id: 8\nevent: agent_run.task_status\ndata: {"task_id":"analysis","status":"completed"}\n\n', final))
    const stream = durableSseReader(reader(meta, 'id: 7\nevent: agent_run.started\ndata: {}\n\n', 'id: 8\nevent: agent_run.task_status\ndata: {"task_id":'), reconnect, new AbortController().signal, async () => {})
    let result = ''
    while (true) { const part = await stream.read(); if (part.done) break; result += new TextDecoder().decode(part.value) }
    expect(reconnect).toHaveBeenCalledExactlyOnceWith(id, '7')
    expect(result.match(/id: 8/g)).toHaveLength(1)
    expect(result).toContain(final)
    expect(result).not.toContain('"task_id":\n')
  })
  it('does not resubmit an unadmitted legacy request', async () => {
    const reconnect = vi.fn()
    const stream = durableSseReader(reader('event: meta\ndata: {"run_id":"run_123"}\n\n'), reconnect, new AbortController().signal)
    await stream.read(); expect((await stream.read()).done).toBe(true)
    expect(reconnect).not.toHaveBeenCalled()
  })
  it('does not resume after final or after user cancellation', async () => {
    const reconnect = vi.fn()
    const controller = new AbortController()
    const stream = durableSseReader(reader(meta, final), reconnect, controller.signal)
    await stream.read(); await stream.read(); expect((await stream.read()).done).toBe(true)
    controller.abort(); await expect(stream.read()).rejects.toMatchObject({ name: 'AbortError' })
    expect(reconnect).not.toHaveBeenCalled()
  })
})
