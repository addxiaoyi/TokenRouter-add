import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref, type Component } from 'vue'
import { defineLazyComponent } from '../asyncComponent'

// asyncComponent.spec.ts verifies optional interface loading through real Vue async
// wrappers. Deferred imports and a failed chunk exercise the visible loading and
// recovery states without fetching application pages or replacing their behavior.

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

afterEach(() => {
  vi.useRealTimers()
})

describe('optional component loading', () => {
  // An unopened panel leaves its loader untouched. Once opened, a slow import shows
  // status before its real component replaces the notice, preserving page interaction.
  it('waits for activation and reports a pending import', async () => {
    vi.useFakeTimers()
    let resolvePanel!: (panel: Component) => void
    const loader = vi.fn(
      () =>
        new Promise<Component>((resolve) => {
          resolvePanel = resolve
        }),
    )
    const visible = ref(false)
    const host = defineComponent({
      components: { Panel: defineLazyComponent(loader) },
      setup: () => ({ visible }),
      template: '<Panel v-if="visible" />',
    })
    const wrapper = mount(host)
    expect(loader).not.toHaveBeenCalled()
    visible.value = true
    await flushPromises()
    await vi.advanceTimersByTimeAsync(150)
    expect(wrapper.get('[role="status"]').text()).toBe('common.loading')
    resolvePanel(defineComponent({ template: '<section>Ready</section>' }))
    await flushPromises()
    expect(wrapper.text()).toBe('Ready')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    wrapper.unmount()
  })

  // A chunk request can fail after a deployment or network interruption. The page
  // retains an accessible error and refresh action rather than silently losing the panel.
  it('offers recovery when an optional chunk fails', async () => {
    const failure = new Error('Chunk download failed')
    const errorHandler = vi.fn()
    const Panel = defineLazyComponent(() => Promise.reject(failure))
    const host = defineComponent({ components: { Panel }, template: '<Panel />' })
    const wrapper = mount(host, { global: { config: { errorHandler } } })
    await flushPromises()
    expect(errorHandler).toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('common.interfaceLoadFailed')
    expect(wrapper.get('button').text()).toBe('common.refresh')
    wrapper.unmount()
  })

  // Closing can beat an unresolved import, and the same request may fail later.
  // Both pending and failure feedback follow visibility while reopening preserves recovery.
  it('hides loading and late failures after a panel closes', async () => {
    vi.useFakeTimers()
    let rejectPanel!: (error: Error) => void
    const active = ref(true)
    const Panel = defineLazyComponent(
      () =>
        new Promise<Component>((_resolve, reject) => {
          rejectPanel = reject
        }),
      () => active.value,
    )
    const host = defineComponent({ components: { Panel }, template: '<Panel />' })
    const wrapper = mount(host, { global: { config: { errorHandler: vi.fn() } } })
    await vi.advanceTimersByTimeAsync(150)
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    active.value = false
    await flushPromises()
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    rejectPanel(new Error('Delayed chunk failure'))
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    active.value = true
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
