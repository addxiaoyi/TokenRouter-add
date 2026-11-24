import type { GroupFormDraft } from './groupFormDraft'

// groupTemplates.ts stores reusable group policies for one administrator in
// the current browser. Records retain selected scalar settings and an explicit
// template name. Reading and writing use the same validation and field selection,
// so saved browser data cannot inject provider IDs or unrelated draft fields.

// Template limits keep the in-browser library small and keep names usable in the selector.
export const MAX_GROUP_TEMPLATES = 20
export const MAX_GROUP_TEMPLATE_NAME = 48

// The form requires a multiplier of at least 0.001, and the database stores six integer digits.
const MIN_MULTIPLIER = 0.001
const MAX_MULTIPLIER = 999_999.9999
const MAX_STORAGE_LENGTH = 16_384

// GroupTemplatePolicy holds the fields reusable without copying routing relationships.
export type GroupTemplatePolicy = Pick<
  GroupFormDraft,
  | 'rate_multiplier'
  | 'is_exclusive'
  | 'session_isolation_enabled'
  | 'require_oauth_only'
  | 'require_privacy_set'
>

// GroupTemplate names a reusable policy shown in the group creation dialog.
export interface GroupTemplate {
  name: string // Visible template name.
  policy: GroupTemplatePolicy // Reusable scalar settings.
}

// groupTemplatePolicy returns a validated, allowlisted projection for storage or
// application to a draft. Invalid multipliers and missing or mistyped switches
// reject the entire projection before any settings can change.
export function groupTemplatePolicy(source: unknown): GroupTemplatePolicy {
  if (source === null || typeof source !== 'object') throw new Error('Invalid group policy')
  const policy = source as Partial<GroupTemplatePolicy>
  const {
    rate_multiplier,
    is_exclusive,
    session_isolation_enabled,
    require_oauth_only,
    require_privacy_set,
  } = policy
  if (
    typeof rate_multiplier !== 'number' ||
    !Number.isFinite(rate_multiplier) ||
    rate_multiplier < MIN_MULTIPLIER ||
    rate_multiplier > MAX_MULTIPLIER ||
    typeof is_exclusive !== 'boolean' ||
    typeof session_isolation_enabled !== 'boolean' ||
    typeof require_oauth_only !== 'boolean' ||
    typeof require_privacy_set !== 'boolean'
  ) {
    throw new Error('Invalid group policy')
  }
  return {
    rate_multiplier,
    is_exclusive,
    session_isolation_enabled,
    require_oauth_only,
    require_privacy_set,
  }
}

// storageKey assigns each eligible account an independent browser storage key.
function storageKey(userId: number): string {
  if (!Number.isSafeInteger(userId) || userId <= 0) throw new Error('Invalid template owner')
  return `tokenrouter:group-templates:v1:${userId}`
}

// normalizeTemplates validates a complete library and trims its names. Duplicate
// names are rejected without case sensitivity, and each policy is projected onto
// recognized fields before returning records for storage or display.
function normalizeTemplates(entries: unknown): GroupTemplate[] {
  if (!Array.isArray(entries) || entries.length > MAX_GROUP_TEMPLATES) {
    throw new Error('Invalid group template library')
  }
  const names = new Set<string>()
  return entries.map((entry: unknown) => {
    if (entry === null || typeof entry !== 'object') throw new Error('Invalid group template')
    const { name, policy } = entry as Partial<GroupTemplate>
    const label = typeof name === 'string' ? name.trim() : ''
    const identity = label.toLowerCase()
    if (!label || label.length > MAX_GROUP_TEMPLATE_NAME || names.has(identity)) {
      throw new Error('Invalid or duplicate group template name')
    }
    names.add(identity)
    return { name: label, policy: groupTemplatePolicy(policy) }
  })
}

// readGroupTemplates reads the active account's library from local browser
// storage. A missing library is empty. Invalid JSON or storage access failures
// propagate so the form can show that saved templates are unavailable.
export function readGroupTemplates(userId: number): GroupTemplate[] {
  const json = localStorage.getItem(storageKey(userId))
  if (json === null) return []
  // Bound the work before parsing possibly corrupted browser storage.
  if (json.length > MAX_STORAGE_LENGTH) throw new Error('Group template library is too large')
  return normalizeTemplates(JSON.parse(json) as unknown)
}

// writeGroupTemplates replaces the account's library with validated records.
// Callers read the latest library and decide which entry to change before this
// call. Browser failures propagate so the controls do not report a successful save.
export function writeGroupTemplates(userId: number, entries: readonly GroupTemplate[]): void {
  localStorage.setItem(storageKey(userId), JSON.stringify(normalizeTemplates(entries)))
}
