import { afterEach, describe, expect, it, vi } from 'vitest'
import { streamChat } from '../chat'

// chat.spec.ts drives the gateway stream parser with controlled response bytes.
// Fixtures split UTF-8 and CRLF event boundaries, interrupt output, and expose
// explicit API failures without contacting a provider or creating billable usage.

afterEach(() => vi.unstubAllGlobals())

// response emits one byte at a time to stress incremental Unicode and SSE decoding.
function response(text: string): Response {
  const bytes = new TextEncoder().encode(text)
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const byte of bytes) controller.enqueue(new Uint8Array([byte]))
        controller.close()
      },
    }),
  )
}

describe('gateway chat streaming', () => {
  // Multibyte content and event delimiters cross arbitrary network chunk boundaries.
  // The parser preserves output, reasoning, and usage while excluding local metadata.
  it('preserves split Unicode, reasoning and usage', async () => {
    const request = vi
      .fn()
      .mockResolvedValue(
        response(
          'data: {"choices":[{"delta":{"content":"你好","reasoning_content":"思考"}}]}\r\n\r\ndata: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":12}}\r\n\r\ndata: [DONE]\r\n\r\n',
        ),
      )
    vi.stubGlobal('fetch', request)
    const emit = vi.fn()
    await streamChat(
      'fixture-key',
      'model',
      [{ role: 'user', content: 'hello', reasoning: 'private local metadata' }],
      new AbortController().signal,
      emit,
    )
    expect(emit).toHaveBeenCalledWith({ content: '你好', reasoning: '思考', tokens: undefined })
    expect(emit).toHaveBeenCalledWith({ content: '', reasoning: '', tokens: 12 })
    expect(JSON.parse(request.mock.calls[0][1].body).messages).toEqual([
      { role: 'user', content: 'hello' },
    ])
  })

  // A provider disconnects after sending text without a completion marker.
  // Already received text remains observable, while completion rejects instead of claiming success.
  it('rejects premature EOF after preserving partial text', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(response('data: {"choices":[{"delta":{"content":"partial"}}]}\n\n')),
    )
    const emit = vi.fn()
    await expect(
      streamChat('fixture', 'model', [], new AbortController().signal, emit),
    ).rejects.toThrow('before completion')
    expect(emit).toHaveBeenCalledWith(expect.objectContaining({ content: 'partial' }))
  })

  // A successful HTTP status can still contain an upstream error event.
  // The caller receives a failure instead of saving an empty successful response.
  it('rejects explicit error events', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(response('data: {"error":{"message":"private upstream details"}}\n\n')),
    )
    await expect(
      streamChat('fixture', 'model', [], new AbortController().signal, vi.fn()),
    ).rejects.toThrow('could not complete')
  })
})
