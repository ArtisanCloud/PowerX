import { describe, expect, it } from 'vitest'
import { isRunStateEvent, isVisibleAssistantContentEvent } from '~/utils/agent/streamEvent'

describe('agent stream event channels', () => {
  it('keeps a run-state final out of the visible assistant response channel', () => {
    expect(isRunStateEvent('agent_run.final')).toBe(true)
    expect(isVisibleAssistantContentEvent('agent_run.final')).toBe(false)
  })

  it('allows only the paired plain final to render the assistant response', () => {
    expect(isRunStateEvent('final')).toBe(false)
    expect(isVisibleAssistantContentEvent('final')).toBe(true)
  })
})
