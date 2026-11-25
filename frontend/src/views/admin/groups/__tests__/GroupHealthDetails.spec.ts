import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupHealthDetails from '../GroupHealthDetails.vue'

// GroupHealthDetails.spec.ts checks group activation, provider-count evidence,
// and direct fallback disclosure through the rendered status component. These
// tests use local snapshots only, ensuring neither positive counts nor an
// unresolved fallback target is presented as a successful live health check.

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
  }),
}))

// groupFixture builds a complete health-row snapshot with no fallback targets.
// Callers override activation or counts to distinguish disabled, empty, limited,
// and candidate-bearing groups without adding unrelated administrative fields.
function groupFixture() {
  return {
    id: 1,
    status: 'active' as const,
    provider_count: 3,
    active_provider_count: 2,
    rate_limited_provider_count: 1,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    unavailable_fallback_group_id: null,
  }
}

describe('Group health evidence', () => {
  // Enabled configuration and a nonzero provider total can coexist with zero
  // eligible snapshot candidates. The component must expose that lack of candidates separately.
  it('separates enabled configuration from an unavailable provider snapshot', () => {
    const wrapper = mount(GroupHealthDetails, {
      props: { group: { ...groupFixture(), active_provider_count: 0 } },
    })
    expect(wrapper.text()).toContain('admin.groups.health.activation.active')
    expect(wrapper.find('[data-health-state="unavailable"]').exists()).toBe(true)
    expect(wrapper.find('[data-health-state="candidates"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.groups.health.snapshotHint')
    wrapper.unmount()
  })

  // A group without any linked providers has a distinct actionable problem.
  // Missing zero-valued API counts must keep that empty snapshot out of candidate-bearing status.
  it('identifies groups without linked providers including omitted zero counts', () => {
    const wrapper = mount(GroupHealthDetails, {
      props: {
        group: { ...groupFixture(), provider_count: undefined, active_provider_count: undefined },
      },
    })
    expect(wrapper.find('[data-health-state="empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  // A disabled group can retain active provider counts from its membership.
  // The saved disabled configuration must take precedence over the candidate count summary.
  it('keeps disabled configuration distinct from its active provider count', () => {
    const wrapper = mount(GroupHealthDetails, {
      props: { group: { ...groupFixture(), status: 'inactive' } },
    })
    expect(wrapper.find('[data-health-state="disabled"]').exists()).toBe(true)
    expect(wrapper.find('[data-health-state="candidates"]').exists()).toBe(false)
    wrapper.unmount()
  })

  // A positive count provides candidates without proving request-specific compatibility.
  // The explanation must retain counts and the explicit snapshot caveat for administrator inspection.
  it('discloses candidate counts with the limits of that evidence', () => {
    const wrapper = mount(GroupHealthDetails, { props: { group: groupFixture() } })
    expect(wrapper.find('[data-health-state="candidates"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('"active":2,"limited":1,"total":3')
    expect(wrapper.text()).toContain('admin.groups.health.snapshotHint')
    expect(wrapper.get('details').attributes('open')).toBeUndefined()
    wrapper.unmount()
  })

  // Each fallback type has its own trigger and stored target. A target absent
  // from loaded choices must remain unverified while known disabled and self-referential targets remain visible.
  it('distinguishes self targets, disabled targets, and unverified targets', () => {
    const wrapper = mount(GroupHealthDetails, {
      props: {
        group: {
          ...groupFixture(),
          fallback_group_id: 1,
          fallback_group_id_on_invalid_request: 2,
          unavailable_fallback_group_id: 3,
        },
        fallbackGroups: [{ id: 2, name: 'Saved target', status: 'inactive' }],
      },
    })
    expect(wrapper.text()).toContain('admin.groups.health.selfFallback')
    expect(wrapper.text()).toContain('admin.groups.health.targetDisabled')
    expect(wrapper.text()).toContain('admin.groups.health.targetUnverified')
    expect(wrapper.text()).toContain('Saved target')
    expect(wrapper.text()).toContain('#3')
    expect(wrapper.text()).toContain('admin.groups.health.fallbackHint')
    wrapper.unmount()
  })
})
