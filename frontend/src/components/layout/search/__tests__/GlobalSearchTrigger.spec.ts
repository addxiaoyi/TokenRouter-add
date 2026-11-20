import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import GlobalSearchTrigger from '../GlobalSearchTrigger.vue'
import { closeGlobalSearch, globalSearchAvailable, globalSearchOpen } from '../globalSearchState'

// GlobalSearchTrigger.spec.ts checks the header search entry: both of its buttons open the
// shared palette state, its shortcut hint follows the platform, and mounting it enables
// the palette's keyboard shortcuts. Translation returns message keys.

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

// mountTrigger renders the trigger with the icon component stubbed.
function mountTrigger() {
  return mount(GlobalSearchTrigger, { global: { stubs: { Icon: true } } })
}

afterEach(() => {
  closeGlobalSearch()
  vi.restoreAllMocks()
})

describe('GlobalSearchTrigger', () => {
  // The palette component does not exist before its first opening. A mounted trigger
  // still admits shortcuts and releases their listener when its header leaves.
  it('opens by keyboard before the palette loads and releases shortcuts on unmount', async () => {
    const wrapper = mountTrigger()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true }))
    expect(globalSearchOpen.value).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true }))
    expect(globalSearchOpen.value).toBe(false)
    wrapper.unmount()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true }))
    expect(globalSearchOpen.value).toBe(false)
  })

  // The field-style button and the narrow-screen icon button both open the palette and
  // describe it as a dialog to assistive technology.
  it('opens the palette from either button', async () => {
    const wrapper = mountTrigger()
    for (const id of ['global-search-trigger', 'global-search-icon-trigger']) {
      closeGlobalSearch()
      const button = wrapper.get(`[data-testid="${id}"]`)
      expect(button.attributes('aria-haspopup')).toBe('dialog')
      await button.trigger('click')
      expect(globalSearchOpen.value).toBe(true)
    }
    wrapper.unmount()
  })

  // The shortcut hint shows Command on Apple platforms and Control elsewhere, matching
  // the keys the palette listens for.
  it('labels the shortcut for the platform', () => {
    vi.spyOn(navigator, 'platform', 'get').mockReturnValue('MacIntel')
    const mac = mountTrigger()
    expect(mac.get('[data-testid="global-search-kbd"]').text()).toBe('⌘K')
    mac.unmount()
    vi.spyOn(navigator, 'platform', 'get').mockReturnValue('Linux x86_64')
    const other = mountTrigger()
    expect(other.get('[data-testid="global-search-kbd"]').text()).toBe('Ctrl K')
    other.unmount()
  })

  // Keyboard shortcuts are available exactly while a trigger is mounted, so pages without
  // a search entry keep "/" and the shortcut keys.
  it('enables shortcuts while mounted', () => {
    expect(globalSearchAvailable.value).toBe(false)
    const wrapper = mountTrigger()
    expect(globalSearchAvailable.value).toBe(true)
    wrapper.unmount()
    expect(globalSearchAvailable.value).toBe(false)
  })
})
