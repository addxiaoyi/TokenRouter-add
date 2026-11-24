import { beforeEach, describe, expect, it } from 'vitest'
import { createGroupFormDraft } from '../groupFormDraft'
import { groupTemplatePolicy, readGroupTemplates, writeGroupTemplates } from '../groupTemplates'

// groupTemplates.spec.ts protects the draft-to-storage boundary for reusable
// group policies. It uses the real creation draft and browser storage to verify
// that copied configuration has the same scope and fields after a second open.

beforeEach(() => localStorage.clear())

describe('group creation templates', () => {
  // Saving an active draft keeps its selected billing and access policy. A
  // different administrator and excluded draft fields remain outside the saved copy.
  it('stores only policy fields under the active account', () => {
    const draft = createGroupFormDraft()
    draft.name = 'Do not store'
    draft.copy_providers_from_group_ids = [42]
    draft.rate_multiplier = 0.25
    draft.is_exclusive = true
    writeGroupTemplates(10, [{ name: 'Example policy', policy: groupTemplatePolicy(draft) }])
    expect(readGroupTemplates(10)).toEqual([
      {
        name: 'Example policy',
        policy: {
          rate_multiplier: 0.25,
          is_exclusive: true,
          session_isolation_enabled: false,
          require_oauth_only: false,
          require_privacy_set: false,
        },
      },
    ])
    expect(readGroupTemplates(11)).toEqual([])
    expect(localStorage.getItem(localStorage.key(0)!)).not.toMatch(/Do not store|copy_providers|42/)
  })

  // A malformed saved multiplier makes the template unavailable before it can
  // overwrite a valid form. Existing group creation remains independent of storage.
  it('rejects invalid saved policy', () => {
    const draft = createGroupFormDraft()
    draft.rate_multiplier = Number.NaN
    expect(() =>
      writeGroupTemplates(10, [{ name: 'Broken policy', policy: groupTemplatePolicy(draft) }]),
    ).toThrow()
    expect(readGroupTemplates(10)).toEqual([])
  })
})
