import type { ChatMessage } from '@/api/chat'

// chatHistory.ts stores browser conversations under an authenticated account
// identifier. Stored messages contain text only; API credentials never belong
// to the persisted shape. Reads validate imported records before exposing them
// to a page, and storage failures remain visible to the caller.
//
// Legacy history used a shared browser key without account identity. Import is
// explicit, preserves source records, and merges unique conversation identifiers
// into the current account. Export carries the same bounded text-only records
// so another browser can restore a conversation without receiving a credential.

// ChatSession holds one local conversation and its presentation metadata.
export interface ChatSession {
  id: string // Stable local conversation identifier.
  title: string // Editable conversation display title.
  model: string // Last selected model identifier.
  createdAt: number // Unix creation time in milliseconds.
  updatedAt: number // Unix update time in milliseconds.
  messages: ChatMessage[] // Ordered conversation text records.
}

// Storage limits retain bounded local history and reject unexpectedly large imports.
const MAX_SESSIONS = 50
const MAX_HISTORY_BYTES = 4 * 1024 * 1024

// historyKey isolates persisted conversations for one positive authenticated user ID.
function historyKey(userID: number): string {
  if (!Number.isSafeInteger(userID) || userID <= 0)
    throw new Error('An authenticated account is required')
  return `tokenrouter-chat-v1:${userID}`
}

// decodeHistory validates and copies a bounded array of portable conversation records.
// Unknown fields are omitted so imported credentials cannot reach storage. Invalid
// records reject the entire import, preserving the caller's existing conversations.
export function decodeHistory(raw: string): ChatSession[] {
  if (raw.length > MAX_HISTORY_BYTES) throw new Error('Conversation history exceeds the size limit')
  const parsed: unknown = JSON.parse(raw)
  if (!Array.isArray(parsed) || parsed.length > MAX_SESSIONS)
    throw new Error('Invalid conversation history')
  return parsed.map((value: unknown) => {
    if (!value || typeof value !== 'object') throw new Error('Invalid conversation')
    const record = value as Record<string, unknown>
    if (
      typeof record.id !== 'string' ||
      !record.id ||
      typeof record.title !== 'string' ||
      typeof record.model !== 'string' ||
      !Array.isArray(record.messages) ||
      record.messages.length > 500 ||
      typeof record.createdAt !== 'number' ||
      typeof record.updatedAt !== 'number' ||
      !Number.isFinite(record.createdAt) ||
      !Number.isFinite(record.updatedAt)
    )
      throw new Error('Invalid conversation record')
    const messages = record.messages.map((message: unknown): ChatMessage => {
      if (!message || typeof message !== 'object') throw new Error('Invalid message')
      const item = message as Record<string, unknown>
      if (
        !['user', 'assistant', 'system'].includes(String(item.role)) ||
        typeof item.content !== 'string'
      )
        throw new Error('Invalid message content')
      return {
        role: item.role as ChatMessage['role'],
        content: item.content,
        ...(typeof item.reasoning === 'string' ? { reasoning: item.reasoning } : {}),
      }
    })
    return {
      id: record.id,
      title: record.title.slice(0, 160),
      model: record.model,
      createdAt: record.createdAt,
      updatedAt: record.updatedAt,
      messages,
    }
  })
}

// readHistory loads validated local conversations for the current authenticated account.
export function readHistory(userID: number): ChatSession[] {
  const raw = localStorage.getItem(historyKey(userID))
  return raw ? decodeHistory(raw) : []
}

// writeHistory persists validated records and reports quota or browser storage failures.
// Callers keep their in-memory draft on failure so it can be exported. The
// fifty most recently updated conversations survive an intentional history limit.
export function writeHistory(userID: number, sessions: ChatSession[]): void {
  const bounded = [...sessions].sort((a, b) => b.updatedAt - a.updatedAt).slice(0, MAX_SESSIONS)
  localStorage.setItem(historyKey(userID), JSON.stringify(decodeHistory(JSON.stringify(bounded))))
}

// readLegacyHistory reads the original site's records only for an explicit import.
// Source values remain unchanged, and no account is inferred from shared storage.
// The current OpenWebUI history takes precedence over the older session format.
export function readLegacyHistory(): ChatSession[] {
  const raw =
    localStorage.getItem('helstare-openwebui-sessions-v2') ??
    sessionStorage.getItem('helstare-chat-sessions-v1') ??
    localStorage.getItem('helstare-chat-sessions-v1')
  return raw ? decodeHistory(raw) : []
}
