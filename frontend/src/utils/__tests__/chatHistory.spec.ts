import { beforeEach, describe, expect, it } from 'vitest'
import {
  decodeHistory,
  readHistory,
  readLegacyHistory,
  writeHistory,
  type ChatSession,
} from '../chatHistory'

// chatHistory.spec.ts verifies account isolation and text-only history persistence.
// Browser storage fixtures cover explicit legacy import, invalid records, and
// removal of unrelated imported fields without contacting a server or exposing keys.

// conversation supplies one portable history record shared by the storage assertions.
const conversation: ChatSession = {
  id: 'one',
  title: 'Example',
  model: 'model',
  createdAt: 1,
  updatedAt: 2,
  messages: [{ role: 'user', content: 'hello' }],
}
beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
})

describe('local conversation history', () => {
  // Two accounts use the same browser but own separate conversation namespaces.
  // Saving the first account's history cannot expose it to the second account.
  it('isolates accounts and strips credential-like imported fields', () => {
    writeHistory(7, [{ ...conversation, key: 'must-not-persist' } as ChatSession])
    expect(readHistory(7)).toEqual([conversation])
    expect(readHistory(8)).toEqual([])
    expect(localStorage.getItem('tokenrouter-chat-v1:7')).not.toContain('must-not-persist')
  })
  // Shared legacy records require an explicit read and retain their original bytes.
  // Normal account loading cannot silently claim the previous site's shared conversations.
  it('imports legacy history explicitly without deleting its source', () => {
    const raw = JSON.stringify([conversation])
    localStorage.setItem('helstare-openwebui-sessions-v2', raw)
    expect(readHistory(7)).toEqual([])
    expect(readLegacyHistory()).toEqual([conversation])
    expect(localStorage.getItem('helstare-openwebui-sessions-v2')).toBe(raw)
  })
  // Invalid message roles and oversized imports cannot enter the history namespace.
  // Existing data remains untouched when validation fails before a storage write.
  it('rejects malformed imports', () => {
    expect(() =>
      decodeHistory(
        JSON.stringify([{ ...conversation, messages: [{ role: 'script', content: 'bad' }] }]),
      ),
    ).toThrow()
    expect(() => decodeHistory(' '.repeat(4 * 1024 * 1024 + 1))).toThrow()
    expect(() => readHistory(0)).toThrow()
  })
})
