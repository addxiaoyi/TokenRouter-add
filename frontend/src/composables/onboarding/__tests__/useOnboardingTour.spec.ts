import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Config, DriveStep, Driver, DriverHook, PopoverDOM } from 'driver.js'

import { useOnboardingTour } from '../useOnboardingTour'

// useOnboardingTour.spec.ts checks routed navigation, explicit skipping, and
// cancelled interactions across onboarding sessions. The real driver exercises
// welcome buttons, while controlled drivers expose delayed callbacks and cleanup.
// User and tour markers distinguish automatic startup suppression from replay.

// Driver, router, and onboarding store fixtures record navigation and lifecycle
// operations while the test controls active step and route state.
const { driverFactory, routerPush, currentRoute, currentUser, onboardingStore } = vi.hoisted(
  () => ({
    driverFactory: vi.fn(),
    routerPush: vi.fn(),
    currentRoute: { value: { fullPath: '/team' } },
    currentUser: { id: 7, role: 'user' },
    onboardingStore: {
      getDriverInstance: vi.fn(() => null),
      setDriverInstance: vi.fn(),
      isDriverActive: vi.fn(() => false),
      setControlMethods: vi.fn(),
      clearControlMethods: vi.fn(),
    },
  }),
)

vi.mock('driver.js', () => ({ driver: driverFactory }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: currentUser }),
}))

// Mounted harnesses and owned drivers are disposed together after each test so
// timers, keyboard capture, and library overlays cannot leak between tour cases.
const harnesses: ReturnType<typeof mount>[] = []
const ownedDrivers: Driver[] = []

// mountTour exposes the composable through a component with real lifecycle hooks.
// Callers choose automatic startup and receive typed controls; teardown retains
// ownership of every harness even when a test explicitly unmounts one earlier.
function mountTour(autoStart = false) {
  let tour!: ReturnType<typeof useOnboardingTour>
  const Harness = defineComponent({
    // setup installs onboarding controls for this test-owned mounted layout.
    setup() {
      tour = useOnboardingTour({ autoStart })
      return tour
    },
    template: '<div />',
  })
  const wrapper = mount(Harness)
  harnesses.push(wrapper)
  return { wrapper, tour }
}

// mockTourDriver records the operations used by the controller and exposes hook
// invocations with current state. Destruction runs the registered callback once,
// mirroring the driver cleanup boundary without unrelated geometry or animation.
function mockTourDriver() {
  let activeIndex = 0
  let active = true
  let config: Config = {}
  const instance = {
    drive: vi.fn((index = 0) => {
      activeIndex = index
    }),
    moveTo: vi.fn((index: number) => {
      activeIndex = index
    }),
    moveNext: vi.fn(() => {
      activeIndex += 1
    }),
    getActiveIndex: vi.fn(() => activeIndex),
    getActiveElement: vi.fn(() => null),
    refresh: vi.fn(),
    isActive: vi.fn(() => active),
    destroy: vi.fn(() => {
      if (!active) return
      active = false
      config.onDestroyed?.(undefined, config.steps?.[activeIndex] ?? {}, {
        config,
        state: { activeIndex },
        driver: instance as unknown as Driver,
      })
    }),
  }
  const driver = instance as unknown as Driver
  ownedDrivers.push(driver)
  driverFactory.mockImplementation((nextConfig: Config) => {
    config = nextConfig
    return driver
  })
  return {
    instance,
    // invoke supplies the selected hook with this fixture's active driver state.
    // Missing elements are forwarded unchanged, while an omitted step uses the
    // configured active step or an empty definition before configuration exists.
    invoke(
      hook: keyof Pick<Config, 'onCloseClick' | 'onHighlightStarted'>,
      element?: Element,
      step?: DriveStep,
    ) {
      return (config[hook] as DriverHook)?.(element, step ?? config.steps?.[activeIndex] ?? {}, {
        config,
        state: { activeIndex },
        driver,
      })
    },
    // renderPopover drives the public rendering hook with its owned driver state.
    renderPopover(popover: PopoverDOM) {
      config.onPopoverRender?.(popover, { config, state: { activeIndex }, driver })
    },
  }
}

// createPopover supplies native buttons and the documented driver popover nodes.
// Tests click the appended Skip control instead of invoking its callback directly.
function createPopover(): PopoverDOM {
  const wrapper = document.createElement('div')
  const footer = document.createElement('footer')
  const footerButtons = document.createElement('div')
  const previousButton = document.createElement('button')
  const nextButton = document.createElement('button')
  const closeButton = document.createElement('button')
  footerButtons.append(previousButton, nextButton)
  footer.append(footerButtons)
  wrapper.append(footer, closeButton)
  document.body.append(wrapper)
  return {
    wrapper,
    footer,
    footerButtons,
    previousButton,
    nextButton,
    closeButton,
    arrow: document.createElement('div'),
    title: document.createElement('h2'),
    description: document.createElement('div'),
    progress: document.createElement('span'),
  }
}

// Release drivers before unmounting harnesses so destruction observes its owning
// layout, then restore clocks and browser spies used by cancellation scenarios.
afterEach(() => {
  ownedDrivers.splice(0).forEach((instance) => instance.destroy())
  harnesses.splice(0).forEach((wrapper) => wrapper.unmount())
  document.body.innerHTML = ''
  vi.useRealTimers()
  vi.restoreAllMocks()
})
vi.mock('@/stores/onboarding', () => ({ useOnboardingStore: () => onboardingStore }))
vi.mock('vue-router', () => ({
  useRouter: () => ({
    currentRoute,
    resolve: (route: {
      path: string // Destination application route path.
      query?: Record<string, string> // Destination route query values.
    }) => ({
      fullPath: route.query?.scope ? `${route.path}?scope=${route.query.scope}` : route.path,
    }),
    push: routerPush,
  }),
}))

describe('useOnboardingTour team flow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    driverFactory.mockReset()
    localStorage.clear()
    currentUser.id = 7
    currentUser.role = 'user'
    currentRoute.value = { fullPath: '/team' }
    routerPush.mockImplementation(
      async (route: {
        path: string // Destination application route path.
        query?: Record<string, string> // Destination route query values.
      }) => {
        currentRoute.value = {
          fullPath: route.query?.scope ? `${route.path}?scope=${route.query.scope}` : route.path,
        }
        await nextTick()
      },
    )
  })

  // A team owner advances through steps that cross team and key routes, then
  // navigates backward. Router calls must complete before the driver highlights
  // the destination step.
  it('在团队、密钥和使用记录步骤之间按目标路由切换', async () => {
    let activeIndex = 0
    let active = true
    let config: any
    const driverInstance = {
      drive: vi.fn((index = 0) => {
        activeIndex = index
      }),
      moveTo: vi.fn((index: number) => {
        activeIndex = index
      }),
      moveNext: vi.fn(() => {
        activeIndex += 1
      }),
      movePrevious: vi.fn(() => {
        activeIndex -= 1
      }),
      getActiveIndex: vi.fn(() => activeIndex),
      getActiveElement: vi.fn(() => null),
      refresh: vi.fn(),
      isActive: vi.fn(() => active),
      destroy: vi.fn(() => {
        active = false
        config?.onDestroyed?.()
      }),
    }
    driverFactory.mockImplementation((nextConfig) => {
      config = nextConfig
      return driverInstance
    })

    // Provide a visible target for every selector so the test observes router
    // sequencing rather than waiting for unrelated page rendering.
    const target = document.createElement('div')
    vi.spyOn(target, 'getBoundingClientRect').mockReturnValue({ height: 40 } as DOMRect)
    const querySelector = vi.spyOn(document, 'querySelector').mockReturnValue(target)

    const Harness = defineComponent({
      // setup exposes tour methods from a mounted Vue component fixture.
      setup() {
        return useOnboardingTour({ autoStart: false })
      },
      template: '<div />',
    })
    const wrapper = mount(Harness)
    await flushPromises()

    const startTeamTour = (wrapper.vm as any).startTeamTour as (options: {
      isOwner: boolean // Current team ownership flag.
      hasTeam: boolean // Current team membership flag.
    }) => void
    startTeamTour({ isOwner: true, hasTeam: true })
    await flushPromises()

    // The driver keeps progress visible while the highlight callback reveals and
    // refreshes the target before further route navigation.
    expect(config.showProgress).toBe(true)

    const reveal = vi.fn()
    target.addEventListener('onboarding-reveal', reveal)
    await config.onHighlightStarted(target, { element: '[data-tour="group-form-multiplier"]' })
    expect(reveal).toHaveBeenCalledOnce()
    expect(driverInstance.refresh).toHaveBeenCalledOnce()

    for (let index = 0; index < 5; index += 1) {
      await config.onNextClick(null, config.steps[index], {
        config,
        state: { activeIndex: index },
      })
    }

    expect(routerPush).toHaveBeenCalledWith({ path: '/keys', query: { scope: 'team' } })
    expect(driverInstance.moveTo).toHaveBeenLastCalledWith(5)

    await config.onPrevClick(null, config.steps[5], { state: { activeIndex: 5 } })
    expect(routerPush).toHaveBeenLastCalledWith({ path: '/team' })
    expect(driverInstance.moveTo).toHaveBeenLastCalledWith(4)

    config.onCloseClick()
    wrapper.unmount()
    querySelector.mockRestore()
  })

  // The real library disables Previous on its first step. A separate enabled
  // Skip button must end that welcome tour, suppress automatic startup on remount,
  // and still allow the user to replay it explicitly.
  it('首步可跳过，刷新不自动重开且仍能主动重播', async () => {
    currentUser.role = 'admin'
    const actual = await vi.importActual<typeof import('driver.js')>('driver.js')
    driverFactory.mockImplementation((config: Config) => {
      const instance = actual.driver({ ...config, animate: false })
      ownedDrivers.push(instance)
      return instance
    })
    const first = mountTour(true)
    await first.tour.startTour()
    await flushPromises()

    const skip = document.querySelector<HTMLButtonElement>('.driver-popover-skip-btn')!
    expect(skip).not.toBeNull()
    expect(skip.disabled).toBe(false)
    expect(skip.textContent).toBe('onboarding.skip')
    expect(document.querySelector('.driver-popover-progress-text')?.textContent).toMatch(
      /^1 \/ \d+$/,
    )
    for (const key of ['Enter', ' ']) {
      const keypress = new KeyboardEvent('keydown', {
        key,
        bubbles: true,
        cancelable: true,
      })
      skip.dispatchEvent(keypress)
      expect(keypress.defaultPrevented).toBe(false)
    }
    skip.click()
    expect(document.querySelector('.driver-popover')).toBeNull()
    expect(localStorage.getItem('onboarding_tour_7_admin_v4_interactive')).toBe('true')
    expect(onboardingStore.setDriverInstance).toHaveBeenLastCalledWith(null)
    first.wrapper.unmount()

    vi.useFakeTimers()
    const reopened = mountTour(true)
    await vi.advanceTimersByTimeAsync(1100)
    expect(driverFactory).toHaveBeenCalledTimes(1)
    reopened.tour.replayTour()
    await flushPromises()
    expect(driverFactory).toHaveBeenCalledTimes(2)
    expect(document.querySelector('.driver-popover-skip-btn')).not.toBeNull()
  })

  // A team guide belongs to the user who started it, independently of the
  // default role tour. Skipping after identity changes must not mark another
  // user's guide or the personal tour as completed.
  it('跳过只保存启动用户对应的团队引导状态', async () => {
    const fixture = mockTourDriver()
    const { tour } = mountTour()
    await tour.startTour(0, 'team', { isOwner: false, hasTeam: false })
    const popover = createPopover()
    fixture.renderPopover(popover)
    currentUser.id = 8
    popover.footerButtons.querySelector<HTMLButtonElement>('.driver-popover-skip-btn')!.click()

    expect(localStorage.getItem('team_guide_7_v1_routed')).toBe('true')
    expect(localStorage.getItem('team_guide_8_v1_routed')).toBeNull()
    expect(tour.hasSeen()).toBe(false)
    expect(fixture.instance.destroy).toHaveBeenCalledOnce()
  })

  // A highlighted target has already scheduled delayed advancement when Skip is
  // pressed. Destruction must restore its tabindex, remove target and keyboard
  // listeners, and cancel the old delay before a new tour begins at that index.
  it('交互等待中跳过会取消延迟和监听，不推进重播引导', async () => {
    vi.useFakeTimers()
    const first = mockTourDriver()
    const { tour } = mountTour()
    await tour.startTour()
    const target = document.createElement('div')
    document.body.append(target)
    await first.invoke('onHighlightStarted', target, {
      element: target,
      popover: { showButtons: ['close'] },
    })
    expect(target.getAttribute('tabindex')).toBe('0')
    const popover = createPopover()
    first.renderPopover(popover)
    target.click()
    expect(vi.getTimerCount()).toBe(1)
    popover.footerButtons.querySelector<HTMLButtonElement>('.driver-popover-skip-btn')!.click()
    await flushPromises()
    expect(target.hasAttribute('tabindex')).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
    const keypress = new KeyboardEvent('keydown', { key: 'ArrowRight', cancelable: true })
    document.dispatchEvent(keypress)
    expect(keypress.defaultPrevented).toBe(false)
    target.click()
    target.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    expect(vi.getTimerCount()).toBe(0)

    const replay = mockTourDriver()
    await tour.startTour()
    await vi.advanceTimersByTimeAsync(1000)
    expect(first.instance.moveTo).not.toHaveBeenCalled()
    expect(replay.instance.moveTo).not.toHaveBeenCalled()
    expect(replay.instance.isActive()).toBe(true)
  })

  // A conditionally rendered target is still missing when the user skips.
  // Cancelling the visibility poll must release its timer and stop the resumed
  // highlight callback from installing listeners after that target appears.
  it('等待目标出现时跳过不再绑定迟到的目标', async () => {
    vi.useFakeTimers()
    const fixture = mockTourDriver()
    const { tour } = mountTour()
    await tour.startTour()
    const highlighting = fixture.invoke('onHighlightStarted', undefined, {
      element: '#late-tour-target',
      popover: { showButtons: ['close'] },
    })
    const popover = createPopover()
    fixture.renderPopover(popover)
    popover.footerButtons.querySelector<HTMLButtonElement>('.driver-popover-skip-btn')!.click()
    await highlighting
    expect(vi.getTimerCount()).toBe(0)

    const target = document.createElement('div')
    target.id = 'late-tour-target'
    document.body.append(target)
    target.click()
    await vi.advanceTimersByTimeAsync(1000)
    expect(target.hasAttribute('tabindex')).toBe(false)
    expect(fixture.instance.refresh).not.toHaveBeenCalled()
    expect(fixture.instance.moveTo).not.toHaveBeenCalled()
  })

  // Business form code can request advancement with its own delay. Skipping
  // during that delay must settle the pending call without advancing a destroyed
  // driver or causing a later replay to move unexpectedly.
  it('跳过会取消业务表单请求的延迟推进', async () => {
    vi.useFakeTimers()
    const fixture = mockTourDriver()
    const { tour } = mountTour()
    await tour.startTour()
    const pendingAdvance = tour.nextStep(500)
    fixture.invoke('onCloseClick')
    await pendingAdvance
    expect(vi.getTimerCount()).toBe(0)
    expect(fixture.instance.moveNext).not.toHaveBeenCalled()
    expect(onboardingStore.setDriverInstance).toHaveBeenLastCalledWith(null)
    expect(tour.hasSeen()).toBe(true)
  })
})
