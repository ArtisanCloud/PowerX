/** 只恢复已受理的 Run；任何情况下都不重发用户输入。 */
export function durableSseReader(
  initial: ReadableStreamDefaultReader<Uint8Array>,
  reconnect: (runId: string, cursor: string) => Promise<ReadableStreamDefaultReader<Uint8Array>>,
  signal: AbortSignal,
  pause: (ms: number) => Promise<void> = (ms) => new Promise((resolve, reject) => {
    if (signal.aborted) { reject(new DOMException('Aborted', 'AbortError')); return }
    const abort = () => { clearTimeout(timer); reject(new DOMException('Aborted', 'AbortError')) }
    const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve() }, ms)
    signal.addEventListener('abort', abort, { once: true })
  }),
) {
  let reader = initial
  let decoder = new TextDecoder()
  let buffer = ''
  let runId = ''
  let cursor = '0'
  let terminal = false
  let retries = 0
  const inspect = (chunk: Uint8Array) => {
    buffer += decoder.decode(chunk, { stream: true })
    buffer = buffer.replace(/\r\n/g, '\n')
    if (buffer.length > 32 * 1024 * 1024) throw new Error('SSE frame exceeds limit')
    let complete = ''
    let boundary: number
    while ((boundary = buffer.indexOf('\n\n')) >= 0) {
      const block = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      complete += block + '\n\n'
      let event = ''
      let eventId = ''
      const data: string[] = []
      for (const line of block.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim()
        else if (line.startsWith('id:')) eventId = line.slice(3).trim()
        else if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
      }
      try {
        const payload = JSON.parse(data.join('\n'))
        if (event === 'meta' && payload.durable_run === true && /^[a-f0-9-]{36}$/i.test(payload.run_id || '')) runId = payload.run_id
      } catch { /* 未完成或非 JSON 帧由主解析器处理。 */ }
      if (runId && /^(0|[1-9]\d*)$/.test(eventId) && BigInt(eventId) > BigInt(cursor)) cursor = eventId
      if (event === 'final' || event === 'end') terminal = true
    }
    return complete
  }
  return {
    async read(): Promise<ReadableStreamReadResult<Uint8Array>> {
      while (true) {
        if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
        let result: ReadableStreamReadResult<Uint8Array> | undefined
        let failure: unknown
        try { result = await reader.read() } catch (error) { failure = error }
        if (result && !result.done) {
          const complete = inspect(result.value)
          if (complete) return { done: false, value: new TextEncoder().encode(complete) }
          continue
        }
        if (!runId || terminal || signal.aborted) {
          if (failure) throw failure
          return { done: true, value: undefined }
        }
        try { reader.releaseLock() } catch { /* 旧连接已关闭 */ }
        let connected = false
        while (retries < 5 && !signal.aborted) {
          await pause(Math.min(1000 * 2 ** retries++, 8000))
          try { reader = await reconnect(runId, cursor); connected = true; break }
          catch (error) { failure = error }
        }
        if (!connected) throw failure || new Error('运行订阅恢复失败，请查看此运行的追踪状态')
        // 不完整事件从未交给主解析器，重连后由服务器从确认游标重放。
        buffer = ''; decoder = new TextDecoder()
      }
    },
    releaseLock() { reader.releaseLock() },
  }
}
