import { buildGatewayUrl } from './url'

// chat.ts sends playground requests through the same authenticated gateway as
// external clients. Keys remain in caller memory and never enter conversation
// persistence. Model discovery observes the selected key's routing restrictions.
// Streaming keeps incomplete UTF-8 and SSE frames until their boundaries arrive.
//
// A terminal marker or finish reason distinguishes completion from a broken
// connection. Cancellation closes the response reader, and bounded buffering
// rejects oversized events before they can consume unlimited browser memory.
// Partial text stays with the caller when later protocol or network work fails.

// ChatMessage carries conversation text and optional local reasoning display.
export interface ChatMessage {
  role: 'user' | 'assistant' | 'system' // Gateway conversation role.
  content: string // Visible conversation text.
  reasoning?: string // Optional model reasoning text.
}

// ChatDelta exposes response fragments without upstream provider diagnostics.
export interface ChatDelta {
  content: string // Incremental visible output.
  reasoning: string // Incremental reasoning output.
  tokens?: number // Total reported token count.
}

// Stream limits bound one event and the complete visible response independently.
const MAX_EVENT_BYTES = 1024 * 1024
const MAX_RESPONSE_CHARACTERS = 1024 * 1024

// readChatModels obtains model identifiers permitted by the selected API key.
// The caller owns cancellation and must discard results after changing keys.
// Failed discovery is reported instead of offering an unrelated catalog.
export async function readChatModels(key: string, signal: AbortSignal): Promise<string[]> {
  const response = await fetch(buildGatewayUrl('/v1/models'), {
    headers: { Authorization: `Bearer ${key}` },
    signal,
    credentials: 'omit',
  })
  if (!response.ok) throw new Error(`Model discovery failed (${response.status})`)
  const body = (await response.json()) as { data?: { id: string }[] }
  if (!Array.isArray(body.data)) throw new Error('Invalid model discovery response')
  return [
    ...new Set(
      body.data.map((item) => item.id).filter((id) => typeof id === 'string' && id.length > 0),
    ),
  ].sort()
}

// streamChat emits text from a billed gateway chat request until completion.
// The caller supplies an active key, a permitted model, and a cancellation
// signal. Wire messages exclude local reasoning and UI metadata. Partial output
// is emitted immediately; failures after output still reject the promise so the
// caller can mark that response incomplete without retrying a charged request.
// Decoder state survives split byte sequences, and SSE fields are interpreted
// only after the whole event arrives, including CRLF-delimited provider streams.
export async function streamChat(
  key: string,
  model: string,
  messages: ChatMessage[],
  signal: AbortSignal,
  emit: (delta: ChatDelta) => void,
): Promise<void> {
  const response = await fetch(buildGatewayUrl('/v1/chat/completions'), {
    method: 'POST',
    credentials: 'omit',
    signal,
    headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      model,
      messages: messages.map(({ role, content }) => ({ role, content })),
      stream: true,
      stream_options: { include_usage: true },
    }),
  })
  if (!response.ok) throw new Error(`Chat request failed (${response.status})`)
  if (!response.body) throw new Error('Chat response stream is missing')
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let pending = ''
  let completed = false
  let characters = 0
  try {
    while (true) {
      const chunk = await reader.read()
      pending += decoder.decode(chunk.value, { stream: !chunk.done })
      if (pending.length > MAX_EVENT_BYTES) throw new Error('Chat event exceeds the size limit')
      let boundary = /\r?\n\r?\n/.exec(pending)
      while (boundary) {
        const event = pending.slice(0, boundary.index)
        pending = pending.slice(boundary.index + boundary[0].length)
        const payload = event
          .split(/\r?\n/)
          .filter((line) => line.startsWith('data:'))
          .map((line) => line.slice(5).trimStart())
          .join('\n')
        if (payload === '[DONE]') {
          completed = true
          return
        }
        if (payload) {
          const value = JSON.parse(payload) as {
            error?: unknown
            choices?: {
              delta?: { content?: string; reasoning_content?: string }
              finish_reason?: string | null
            }[]
            usage?: { total_tokens?: number }
          }
          if (value.error) throw new Error('The model could not complete the response')
          const choice = value.choices?.[0]
          const content = typeof choice?.delta?.content === 'string' ? choice.delta.content : ''
          const reasoning =
            typeof choice?.delta?.reasoning_content === 'string'
              ? choice.delta.reasoning_content
              : ''
          characters += content.length + reasoning.length
          if (characters > MAX_RESPONSE_CHARACTERS)
            throw new Error('Chat response exceeds the size limit')
          if (choice?.finish_reason === 'length' || choice?.finish_reason === 'content_filter')
            throw new Error('Chat response was truncated')
          if (choice?.finish_reason) completed = true
          emit({ content, reasoning, tokens: value.usage?.total_tokens })
        }
        boundary = /\r?\n\r?\n/.exec(pending)
      }
      if (chunk.done) break
    }
    if (!completed || pending.trim()) throw new Error('Chat stream ended before completion')
  } finally {
    // Closing the reader also stops provider output after a terminal marker or
    // parser failure; callers retain fragments already emitted before this cleanup.
    try {
      await reader.cancel()
    } finally {
      reader.releaseLock()
    }
  }
}
